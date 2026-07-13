package core

import (
	"fmt"
	"strings"
)

func MatchRule(rule Rule, torrent Torrent) RuleMatch {
	if !rule.Enabled {
		return RuleMatch{Rule: rule, Torrent: torrent, Matched: false, Reason: "rule disabled"}
	}
	if len(rule.SiteIDs) > 0 && !containsFold(rule.SiteIDs, torrent.SiteID) {
		return RuleMatch{Rule: rule, Torrent: torrent, Matched: false, Reason: "site not included"}
	}
	if len(rule.Categories) > 0 && !containsFold(rule.Categories, torrent.Category) {
		return RuleMatch{Rule: rule, Torrent: torrent, Matched: false, Reason: "category not included"}
	}
	if len(rule.Tags) > 0 {
		matched := false
		for _, tag := range rule.Tags {
			if containsFold(torrent.Tags, tag) || containsFold(torrent.TagIDs, tag) {
				matched = true
				break
			}
		}
		if !matched {
			return RuleMatch{Rule: rule, Torrent: torrent, Matched: false, Reason: "tag not included"}
		}
	}
	title := strings.ToLower(torrent.Title + " " + torrent.Subtitle + " " + torrent.Description + " " + torrent.DetailDescription)
	if rule.Include != "" && !strings.Contains(title, strings.ToLower(rule.Include)) {
		return RuleMatch{Rule: rule, Torrent: torrent, Matched: false, Reason: fmt.Sprintf("include %q not matched", rule.Include)}
	}
	if rule.Exclude != "" && strings.Contains(title, strings.ToLower(rule.Exclude)) {
		return RuleMatch{Rule: rule, Torrent: torrent, Matched: false, Reason: fmt.Sprintf("exclude %q matched", rule.Exclude)}
	}
	if rule.Promotion != "" && !strings.EqualFold(rule.Promotion, torrent.Promotion) {
		return RuleMatch{Rule: rule, Torrent: torrent, Matched: false, Reason: "promotion not matched"}
	}
	if rule.MinSize > 0 && torrent.SizeBytes < rule.MinSize {
		return RuleMatch{Rule: rule, Torrent: torrent, Matched: false, Reason: "size below minimum"}
	}
	if rule.MaxSize > 0 && torrent.SizeBytes > rule.MaxSize {
		return RuleMatch{Rule: rule, Torrent: torrent, Matched: false, Reason: "size above maximum"}
	}
	if rule.MinSeeders > 0 && torrent.Seeders < rule.MinSeeders {
		return RuleMatch{Rule: rule, Torrent: torrent, Matched: false, Reason: "seeders below minimum"}
	}
	return RuleMatch{Rule: rule, Torrent: torrent, Matched: true, Reason: "matched"}
}

func containsFold(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(needle)) {
			return true
		}
	}
	return false
}
