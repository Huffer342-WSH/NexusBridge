// series_episode_test.go 使用临时空文件验证嵌套动漫目录的扫描与集数识别。
package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
)

// TestSeriesEpisodeDetectionInPerEpisodeDirectories 验证同名单集目录不会拆散剧集识别分组。
func TestSeriesEpisodeDetectionInPerEpisodeDirectories(t *testing.T) {
	root := t.TempDir()
	animeRoot := filepath.Join(root, "downloads", "动漫")
	type episodeExpectation struct {
		number  int
		version int
		label   string
	}
	expected := make(map[string]episodeExpectation)
	addEpisode := func(showRoot, stem, extension string, wrapped bool, want episodeExpectation) {
		t.Helper()
		path := filepath.Join(showRoot, stem+extension)
		if wrapped {
			path = filepath.Join(showRoot, stem, stem+extension)
		}
		createFakeSeriesFile(t, path)
		expected[stem+extension] = want
	}

	yaniRoot := filepath.Join(animeRoot, "尼古喵喵")
	episodes := []struct {
		token   string
		number  int
		version int
		label   string
	}{
		{token: "01", number: 1, label: "第 01 集"},
		{token: "02", number: 2, label: "第 02 集"},
		{token: "03", number: 3, label: "第 03 集"},
		{token: "04", number: 4, label: "第 04 集"},
		{token: "05", number: 5, label: "第 05 集"},
		{token: "06v2", number: 6, version: 2, label: "第 06 集 · v2"},
		{token: "07", number: 7, label: "第 07 集"},
	}
	for _, episode := range episodes {
		stem := fmt.Sprintf("[LoliHouse] Yani Neko - %s [WebRip 1080p HEVC-10bit AAC SRTx2]", episode.token)
		addEpisode(yaniRoot, stem, ".mkv", true, episodeExpectation{
			number: episode.number, version: episode.version, label: episode.label,
		})
		createFakeSeriesFile(t, filepath.Join(yaniRoot, stem, "cover.jpg"))
		createFakeSeriesFile(t, filepath.Join(yaniRoot, stem, stem+".ass"))
	}

	// “碧蓝之海 第三季”同时存在同名单集目录和直接文件，两种结构应合并识别。
	grandBlueRoot := filepath.Join(animeRoot, "碧蓝之海 第三季")
	for episode := 1; episode <= 4; episode++ {
		token := fmt.Sprintf("%02d", episode)
		stem := fmt.Sprintf("[LoliHouse] Grand Blue S3 - %s [WebRip 1080p HEVC-10bit AAC ASSx2]", token)
		addEpisode(grandBlueRoot, stem, ".mkv", episode <= 2, episodeExpectation{
			number: episode, label: "第 " + token + " 集",
		})
	}

	// “无职转生 第三季”验证另一组同名单集目录不会与其他动漫混组。
	mushokuRoot := filepath.Join(animeRoot, "无职转生 第三季")
	for episode := 1; episode <= 5; episode++ {
		token := fmt.Sprintf("%02d", episode)
		stem := fmt.Sprintf("[LoliHouse] Mushoku Tensei S3 - %s [WebRip 1080p HEVC-10bit AAC SRTx2]", token)
		addEpisode(mushokuRoot, stem, ".mkv", true, episodeExpectation{
			number: episode, label: "第 " + token + " 集",
		})
	}

	// “死神 千年血戰篇”当前只有一个视频，不能因为同名包装目录而误识别。
	bleachStem := "[Dynamis One] BLEACH 死神 千年血戰篇-禍進譚- - 41 (Baha 1920x1080 AVC AAC MP4) [2E54263A]"
	createFakeSeriesFile(t, filepath.Join(animeRoot, "死神 千年血戰篇", bleachStem, bleachStem+".mp4"))

	for index := 0; index < 12; index++ {
		createFakeSeriesFile(t, filepath.Join(yaniRoot, fmt.Sprintf("image-%02d.jpg", index)))
	}

	sitesDir := filepath.Join(root, "sites")
	if err := os.MkdirAll(sitesDir, 0o755); err != nil {
		t.Fatalf("create fake sites directory: %v", err)
	}
	cfg := config.Default()
	cfg.Storage.Path = filepath.Join(root, "metadata.db")
	cfg.SitesDir = sitesDir
	cfg.QBittorrent.AutoSync = false
	app, err := core.NewApp(t.Context(), cfg)
	if err != nil {
		t.Fatalf("create fake series app: %v", err)
	}
	defer app.Close()

	detail, err := app.SaveSeries(t.Context(), "", core.SeriesSaveRequest{
		Name:                   "动漫目录回归测试",
		Directories:            []string{animeRoot},
		EpisodeNumberDetection: true,
	})
	if err != nil {
		t.Fatalf("save and scan fake series: %v", err)
	}
	if detail.VideoCount != len(expected)+1 {
		t.Fatalf("video count = %d, want %d; images and subtitles must be ignored", detail.VideoCount, len(expected)+1)
	}

	matched := 0
	for _, video := range detail.Videos {
		want, ok := expected[video.Name]
		if !ok {
			if video.EpisodeNumber != nil || video.EpisodeLabel != "" {
				t.Fatalf("other anime video was incorrectly recognized: %#v", video)
			}
			continue
		}
		matched++
		if video.EpisodeNumber == nil || *video.EpisodeNumber != want.number {
			t.Errorf("%s episode number = %v, want %d", video.Name, video.EpisodeNumber, want.number)
		}
		if video.EpisodeVersion != want.version {
			t.Errorf("%s episode version = %d, want %d", video.Name, video.EpisodeVersion, want.version)
		}
		if video.EpisodeLabel != want.label {
			t.Errorf("%s episode label = %q, want %q", video.Name, video.EpisodeLabel, want.label)
		}
	}
	if matched != len(expected) {
		t.Fatalf("matched %d recognized videos, want %d", matched, len(expected))
	}
}

func createFakeSeriesFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fake series directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("fake"), 0o600); err != nil {
		t.Fatalf("create fake series file: %v", err)
	}
}
