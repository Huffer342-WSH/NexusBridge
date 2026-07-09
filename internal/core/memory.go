package core

import (
	"context"
	"fmt"
	"time"

	"nexusbridge/internal/config"
)

type MemoryCatalog struct {
	sites []Site
}

func NewMemoryCatalog(cfg config.Config) *MemoryCatalog {
	return &MemoryCatalog{sites: []Site{}}
}

func (c *MemoryCatalog) ListSites(ctx context.Context) ([]Site, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	sites := make([]Site, len(c.sites))
	copy(sites, c.sites)
	return sites, nil
}

// GetSiteCredential 返回内存目录的空凭据。
func (c *MemoryCatalog) GetSiteCredential(ctx context.Context, siteID string) (SiteCredential, error) {
	return SiteCredential{}, fmt.Errorf("site credentials are not available in memory catalog")
}

// SaveSiteCredential 拒绝保存内存目录凭据。
func (c *MemoryCatalog) SaveSiteCredential(ctx context.Context, credential SiteCredential) (SiteCredential, error) {
	return SiteCredential{}, fmt.Errorf("site credentials are not available in memory catalog")
}

func (c *MemoryCatalog) ListTorrents(ctx context.Context, query TorrentQuery) ([]Torrent, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	now := time.Now().UTC()
	if len(c.sites) == 0 {
		return []Torrent{}, nil
	}
	return []Torrent{
		{
			ID:          "placeholder",
			SiteID:      c.sites[0].ID,
			Title:       "Placeholder torrent until parser/cache milestones land",
			DetailURL:   c.sites[0].BaseURL,
			FirstSeenAt: now,
			LastSeenAt:  now,
		},
	}, nil
}

func (c *MemoryCatalog) FetchSite(ctx context.Context, siteID string) (FetchResult, error) {
	select {
	case <-ctx.Done():
		return FetchResult{}, ctx.Err()
	default:
	}
	for _, site := range c.sites {
		if site.ID == siteID {
			return FetchResult{SiteID: site.ID, Status: "not_implemented"}, nil
		}
	}
	return FetchResult{}, fmt.Errorf("site %q not found", siteID)
}
