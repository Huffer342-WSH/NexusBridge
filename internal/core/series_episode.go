package core

import (
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	minimumEpisodeGroupSize       = 3
	minimumEpisodeClusterRatio    = 0.7
	minimumEpisodeNameSimilarity  = 0.84
	minimumEpisodeSequenceGapRate = 0.8
	maximumEpisodeNumber          = 9999
)

var (
	seriesDigitPattern           = regexp.MustCompile(`[0-9]+`)
	seriesVersionPattern         = regexp.MustCompile(`(?i)v[0-9]+`)
	seriesVersionAfterEpisode    = regexp.MustCompile(`(?i)^[\s._-]*v([0-9]+)`)
	seriesSkeletonVersionPattern = regexp.MustCompile(`(?i)episode[\s._-]*v#+`)
)

type episodeNameCandidate struct {
	videoIndex int
	stem       string
	masked     string
	token      []int
	number     int
	rawNumber  string
	version    int
	skeleton   string
}

type episodeDetectionResult struct {
	matches map[int]episodeNameCandidate
	score   float64
}

// detectSeriesEpisodeNumbers 只为高相似且数字近连续的文件组补充集数标签。
func detectSeriesEpisodeNumbers(videos []SeriesVideo) {
	groups := make(map[string][]int)
	for index, video := range videos {
		parent := seriesEpisodeGroupParent(video)
		key := normalizedFilesystemPath(video.DirectoryPath) + "\x00" + normalizedFilesystemPath(parent)
		groups[key] = append(groups[key], index)
	}
	for _, indexes := range groups {
		if len(indexes) < minimumEpisodeGroupSize {
			continue
		}
		result := detectEpisodeGroup(videos, indexes)
		for videoIndex, match := range result.matches {
			number := match.number
			videos[videoIndex].EpisodeNumber = &number
			videos[videoIndex].EpisodeVersion = match.version
			videos[videoIndex].EpisodeLabel = "第 " + match.rawNumber + " 集"
			if match.version > 0 {
				videos[videoIndex].EpisodeLabel += " · v" + strconv.Itoa(match.version)
			}
		}
	}
}

// seriesEpisodeGroupParent 折叠“同名目录包裹同名视频”的单集下载目录。
func seriesEpisodeGroupParent(video SeriesVideo) string {
	parent := filepath.Dir(video.RelativePath)
	if parent == "." {
		return parent
	}
	stem := strings.TrimSuffix(video.Name, filepath.Ext(video.Name))
	if strings.EqualFold(filepath.Base(parent), stem) {
		return filepath.Dir(parent)
	}
	return parent
}

func detectEpisodeGroup(videos []SeriesVideo, indexes []int) episodeDetectionResult {
	names := make([]episodeNameCandidate, 0, len(indexes))
	maximumTokens := 0
	for _, videoIndex := range indexes {
		name := videos[videoIndex].Name
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		masked := maskSeriesVersions(stem)
		tokens := seriesDigitPattern.FindAllStringIndex(masked, -1)
		if len(tokens) > maximumTokens {
			maximumTokens = len(tokens)
		}
		names = append(names, episodeNameCandidate{videoIndex: videoIndex, stem: stem, masked: masked, token: nil})
		names[len(names)-1].token = flattenTokenRanges(tokens)
	}

	best := episodeDetectionResult{}
	for ordinal := 0; ordinal < maximumTokens; ordinal++ {
		candidates := make([]episodeNameCandidate, 0, len(names))
		for _, name := range names {
			token := tokenRange(name.token, ordinal)
			if token == nil {
				continue
			}
			rawNumber := name.stem[token[0]:token[1]]
			number, err := strconv.Atoi(rawNumber)
			if err != nil || number > maximumEpisodeNumber {
				continue
			}
			candidate := name
			candidate.token = token
			candidate.number = number
			candidate.rawNumber = rawNumber
			candidate.version = episodeVersion(name.stem[token[1]:])
			candidate.skeleton = episodeSkeleton(name.masked, token)
			candidates = append(candidates, candidate)
		}
		result := bestEpisodeCluster(candidates, len(indexes))
		if result.score > best.score {
			best = result
		}
	}
	return best
}

func bestEpisodeCluster(candidates []episodeNameCandidate, groupSize int) episodeDetectionResult {
	required := int(float64(groupSize)*minimumEpisodeClusterRatio + 0.999999)
	if required < minimumEpisodeGroupSize {
		required = minimumEpisodeGroupSize
	}
	best := episodeDetectionResult{}
	for _, seed := range candidates {
		cluster := make([]episodeNameCandidate, 0, len(candidates))
		totalSimilarity := 0.0
		for _, candidate := range candidates {
			similarity := stringSimilarity(seed.skeleton, candidate.skeleton)
			if similarity < minimumEpisodeNameSimilarity {
				continue
			}
			cluster = append(cluster, candidate)
			totalSimilarity += similarity
		}
		if len(cluster) < required || !episodeNumbersAreSequential(cluster) {
			continue
		}
		score := float64(len(cluster))*100 + totalSimilarity/float64(len(cluster))
		if score <= best.score {
			continue
		}
		matches := make(map[int]episodeNameCandidate, len(cluster))
		for _, candidate := range cluster {
			matches[candidate.videoIndex] = candidate
		}
		best = episodeDetectionResult{matches: matches, score: score}
	}
	return best
}

func episodeNumbersAreSequential(candidates []episodeNameCandidate) bool {
	unique := make(map[int]struct{}, len(candidates))
	for _, candidate := range candidates {
		unique[candidate.number] = struct{}{}
	}
	minimumDistinct := (len(candidates) + 1) / 2
	if minimumDistinct < 2 {
		minimumDistinct = 2
	}
	if len(unique) < minimumDistinct {
		return false
	}
	values := make([]int, 0, len(unique))
	for value := range unique {
		values = append(values, value)
	}
	sort.Ints(values)
	goodGaps := 0
	for index := 1; index < len(values); index++ {
		gap := values[index] - values[index-1]
		if gap >= 1 && gap <= 3 {
			goodGaps++
		}
	}
	gapCount := len(values) - 1
	if float64(goodGaps)/float64(gapCount) < minimumEpisodeSequenceGapRate {
		return false
	}
	maximumRange := len(values) * 3
	if maximumRange < 6 {
		maximumRange = 6
	}
	return values[len(values)-1]-values[0] <= maximumRange
}

func maskSeriesVersions(value string) string {
	masked := []byte(value)
	for _, match := range seriesVersionPattern.FindAllStringIndex(value, -1) {
		for index := match[0] + 1; index < match[1]; index++ {
			masked[index] = '#'
		}
	}
	return string(masked)
}

func episodeSkeleton(masked string, token []int) string {
	value := masked[:token[0]] + " episode " + masked[token[1]:]
	value = seriesSkeletonVersionPattern.ReplaceAllString(value, "episode")
	var normalized strings.Builder
	separator := false
	for _, character := range strings.ToLower(value) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || character == '#' {
			if separator && normalized.Len() > 0 {
				normalized.WriteByte(' ')
			}
			normalized.WriteRune(character)
			separator = false
			continue
		}
		separator = true
	}
	return normalized.String()
}

func episodeVersion(suffix string) int {
	match := seriesVersionAfterEpisode.FindStringSubmatch(suffix)
	if len(match) != 2 {
		return 0
	}
	version, _ := strconv.Atoi(match[1])
	return version
}

func flattenTokenRanges(ranges [][]int) []int {
	result := make([]int, 0, len(ranges)*2)
	for _, token := range ranges {
		result = append(result, token[0], token[1])
	}
	return result
}

func tokenRange(flattened []int, ordinal int) []int {
	index := ordinal * 2
	if index < 0 || index+1 >= len(flattened) {
		return nil
	}
	return flattened[index : index+2]
}

func stringSimilarity(left, right string) float64 {
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	maximumLength := len(leftRunes)
	if len(rightRunes) > maximumLength {
		maximumLength = len(rightRunes)
	}
	if maximumLength == 0 {
		return 1
	}
	return 1 - float64(levenshteinDistance(leftRunes, rightRunes))/float64(maximumLength)
}

func levenshteinDistance(left, right []rune) int {
	if len(left) > len(right) {
		left, right = right, left
	}
	previous := make([]int, len(left)+1)
	for index := range previous {
		previous[index] = index
	}
	for rightIndex, rightCharacter := range right {
		current := make([]int, len(left)+1)
		current[0] = rightIndex + 1
		for leftIndex, leftCharacter := range left {
			cost := 0
			if leftCharacter != rightCharacter {
				cost = 1
			}
			current[leftIndex+1] = min(
				current[leftIndex]+1,
				previous[leftIndex+1]+1,
				previous[leftIndex]+cost,
			)
		}
		previous = current
	}
	return previous[len(left)]
}
