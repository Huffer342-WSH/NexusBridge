package core

import "nexusbridge/internal/stringutil"

func torrentKey(torrent Torrent) string {
	return torrent.SiteID + ":" + torrent.ID
}

func firstNonEmpty(values ...string) string {
	return stringutil.FirstNonEmpty(values...)
}
