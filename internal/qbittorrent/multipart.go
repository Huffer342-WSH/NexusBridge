package qbittorrent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (c *Client) postMultipart(ctx context.Context, path string, values url.Values, files []torrentFile) (torrentAddResult, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return torrentAddResult{}, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, fieldValues := range values {
		for _, value := range fieldValues {
			if err := writer.WriteField(name, value); err != nil {
				return torrentAddResult{}, err
			}
		}
	}
	for _, file := range files {
		part, err := writer.CreateFormFile("torrents", file.Name)
		if err != nil {
			return torrentAddResult{}, err
		}
		if _, err := part.Write(file.Data); err != nil {
			return torrentAddResult{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return torrentAddResult{}, err
	}
	resp, err := c.doAuthed(ctx, http.MethodPost, path, &body, writer.FormDataContentType())
	if err != nil {
		return torrentAddResult{}, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	slog.Info("qb add torrent response", "status", resp.StatusCode, "body", strings.TrimSpace(string(respBody)))
	if resp.StatusCode == http.StatusConflict {
		return torrentAddResult{}, fmt.Errorf("%w: %s", ErrTorrentConflict, trimBody(respBody))
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return torrentAddResult{}, fmt.Errorf("qbittorrent torrents/add status %d: %s", resp.StatusCode, trimBody(respBody))
	}
	var addResult torrentAddResult
	if json.Unmarshal(respBody, &addResult) == nil && addResult.FailureCount > 0 {
		return addResult, fmt.Errorf("qbittorrent add failed: %s", trimBody(respBody))
	}
	if text := strings.TrimSpace(string(respBody)); text != "" && !strings.Contains(strings.ToLower(text), "ok") {
		if !strings.Contains(text, "pending_count") && !strings.Contains(text, "success_count") {
			return addResult, fmt.Errorf("qbittorrent add failed: %s", text)
		}
	}
	return addResult, nil
}

func (c *Client) addOptionsForm(opts AddOptions) (url.Values, error) {
	values := url.Values{}
	if opts.SavePath != "" {
		values.Set("savepath", opts.SavePath)
	}
	if opts.Category != "" {
		values.Set("category", opts.Category)
	}
	if len(opts.Tags) > 0 {
		values.Set("tags", strings.Join(nonEmptyStrings(opts.Tags), ","))
	}
	if opts.SkipChecking {
		values.Set("skip_checking", "true")
	}
	if opts.Paused {
		values.Set("paused", "true")
	}
	if opts.RootFolder != nil {
		values.Set("root_folder", strconv.FormatBool(*opts.RootFolder))
	}
	if opts.Rename != "" {
		values.Set("rename", opts.Rename)
	}
	if opts.UploadLimit > 0 {
		values.Set("upLimit", strconv.Itoa(opts.UploadLimit))
	}
	if opts.DownloadLimit > 0 {
		values.Set("dlLimit", strconv.Itoa(opts.DownloadLimit))
	}
	if opts.RatioLimit != nil {
		values.Set("ratioLimit", strconv.FormatFloat(*opts.RatioLimit, 'f', -1, 64))
	}
	if opts.SeedingTimeLimit != nil {
		values.Set("seedingTimeLimit", strconv.Itoa(*opts.SeedingTimeLimit))
	}
	if opts.AutoTMM != nil {
		values.Set("autoTMM", strconv.FormatBool(*opts.AutoTMM))
	}
	if opts.SequentialDownload {
		values.Set("sequentialDownload", "true")
	}
	if opts.FirstLastPiecePrio {
		values.Set("firstLastPiecePrio", "true")
	}
	return values, nil
}

type torrentFile struct {
	Name string
	Data []byte
}
