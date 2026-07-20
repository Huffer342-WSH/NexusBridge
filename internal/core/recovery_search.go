package core

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"nexusbridge/internal/parser"
	"nexusbridge/internal/storage"
)

func (a *App) recoverySiteIDs(requested []string) ([]string, error) {
	if len(requested) == 0 {
		return append([]string(nil), a.siteIDs...), nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(requested))
	for _, siteID := range requested {
		siteID = strings.TrimSpace(siteID)
		if siteID == "" {
			continue
		}
		if _, err := a.findSite(siteID); err != nil {
			return nil, err
		}
		if _, exists := seen[siteID]; exists {
			continue
		}
		seen[siteID] = struct{}{}
		result = append(result, siteID)
	}
	return result, nil
}

func (a *App) searchRecoverySite(ctx context.Context, siteID, keyword string, targetTotalSize int64) ([]Torrent, RecoverySearchAttempt) {
	attempt := RecoverySearchAttempt{SiteID: siteID, Status: "failed"}
	site, err := a.findSite(siteID)
	if err != nil {
		attempt.Error = err.Error()
		return nil, attempt
	}
	records := make([]storage.TorrentRecord, 0)
	seen := map[storage.TorrentKey]struct{}{}
	attempted := make([]string, 0)
	errors := make([]string, 0)
	succeeded := 0
	for _, candidateKeyword := range recoverySearchKeywords(keyword) {
		attempted = append(attempted, candidateKeyword)
		targetURL, err := recoverySearchURL(site, candidateKeyword)
		if err != nil {
			errors = append(errors, candidateKeyword+": "+err.Error())
			continue
		}
		result, err := a.fetchSiteResource(ctx, site, targetURL, siteRequestOptions{
			Timeout: defaultSiteRequestTimeout, RequireCookies: true, LogRequest: true,
		})
		if err != nil {
			errors = append(errors, candidateKeyword+": "+err.Error())
			continue
		}
		parsed, err := parser.ParsePageWithDefinition(result.Body, site.Definition)
		if err != nil {
			errors = append(errors, candidateKeyword+": "+err.Error())
			continue
		}
		succeeded++
		for index, entry := range parsed.Torrents {
			record := recordFromParser(entry, index)
			key := storage.TorrentKey{SiteID: record.SiteID, TorrentID: record.TorrentID}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			records = append(records, record)
		}
	}
	attempt.Keyword = strings.Join(attempted, " | ")
	if succeeded == 0 {
		attempt.Error = strings.Join(errors, "; ")
		return nil, attempt
	}
	if _, err := a.store.UpsertTorrents(ctx, records); err != nil {
		attempt.Error = err.Error()
		return nil, attempt
	}
	allTorrents := make([]Torrent, 0, len(records))
	torrents := make([]Torrent, 0, len(records))
	a.mu.Lock()
	for _, record := range records {
		torrent := torrentFromRecord(record)
		allTorrents = append(allTorrents, torrent)
		if recoverySiteSizeCompatible(torrent.SizeBytes, targetTotalSize) {
			torrents = append(torrents, torrent)
		}
		a.cache[torrentKey(torrent)] = torrent
	}
	a.mu.Unlock()
	attempt.Candidates = len(allTorrents)
	attempt.FilesSaved, attempt.FilesFailed, err = a.ensureTorrentFiles(ctx, torrents)
	if err != nil {
		attempt.Error = err.Error()
		return torrents, attempt
	}
	attempt.Status = "ok"
	if len(errors) > 0 {
		attempt.Error = strings.Join(errors, "; ")
	}
	return torrents, attempt
}

func recoverySiteSizeCompatible(reported, target int64) bool {
	if reported <= 0 || target <= 0 {
		return true
	}
	difference := reported - target
	if difference < 0 {
		difference = -difference
	}
	tolerance := target / 20
	if tolerance < 1<<20 {
		tolerance = 1 << 20
	}
	return difference <= tolerance
}

var recoveryBracketPattern = regexp.MustCompile(`\[([^\]]+)\]`)

func recoverySearchKeywords(folderName string) []string {
	result := []string{}
	seen := map[string]struct{}{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || len([]rune(value)) < 4 {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	add(folderName)
	add(strings.TrimSuffix(folderName, filepath.Ext(folderName)))
	segments := []string{}
	for _, match := range recoveryBracketPattern.FindAllStringSubmatch(folderName, -1) {
		if len(match) > 1 {
			segments = append(segments, strings.TrimSpace(match[1]))
		}
	}
	sort.SliceStable(segments, func(i, j int) bool { return len([]rune(segments[i])) > len([]rune(segments[j])) })
	for _, segment := range segments {
		prefix := segment
		if index := strings.IndexAny(prefix, "~～〜"); index >= 0 {
			prefix = prefix[:index]
		}
		add(prefix)
		add(segment)
	}
	parts := strings.FieldsFunc(folderName, func(r rune) bool {
		return strings.ContainsRune("[](){}<>_- .~～〜/\\", r)
	})
	sort.SliceStable(parts, func(i, j int) bool { return len([]rune(parts[i])) > len([]rune(parts[j])) })
	for _, part := range parts {
		add(part)
	}
	runes := []rune(strings.TrimSpace(folderName))
	if len(runes) > 24 {
		add(string(runes[:24]))
	}
	const maxRecoverySearchKeywords = 5
	if len(result) > maxRecoverySearchKeywords {
		result = result[:maxRecoverySearchKeywords]
	}
	return result
}

func recoverySearchURL(site runtimeSite, keyword string) (string, error) {
	definition := site.Definition.HTML.Search
	if len(definition.Paths) > 0 && definition.Paths[0].Method != "" && !strings.EqualFold(definition.Paths[0].Method, "get") {
		return "", fmt.Errorf("site %s recovery search only supports GET", site.ID)
	}
	parsed, err := url.Parse(parser.SiteConfigFromDefinition(site.Definition).URL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	keywordApplied := false
	for name, raw := range definition.Params {
		value := fmt.Sprint(raw)
		for _, token := range []string{"{{keyword}}", "{keyword}", "{{value}}"} {
			if strings.Contains(value, token) {
				value = strings.ReplaceAll(value, token, keyword)
				keywordApplied = true
			}
		}
		query.Set(name, value)
	}
	if !keywordApplied {
		name := "search"
		if definition.Fields.Keyword != nil && strings.TrimSpace(definition.Fields.Keyword.Name) != "" {
			name = definition.Fields.Keyword.Name
		}
		query.Set(name, keyword)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func sortRecoveryMatches(matches []RecoveryMatch) {
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Torrent.SiteID != matches[j].Torrent.SiteID {
			return matches[i].Torrent.SiteID < matches[j].Torrent.SiteID
		}
		return matches[i].Torrent.ID < matches[j].Torrent.ID
	})
}
