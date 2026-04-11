package transformers

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	youtube "github.com/kkdai/youtube/v2"

	"github.com/kriswong/corticalstack/internal/integrations"
	"github.com/kriswong/corticalstack/internal/pipeline"
)

// YouTubeTransformer fetches a YouTube video's metadata + captions via
// github.com/kkdai/youtube/v2. If no captions are available it downloads
// the lowest-bitrate audio stream and pushes it through Deepgram.
type YouTubeTransformer struct {
	// Deepgram is optional. When nil, videos with no captions fail rather
	// than silently producing an empty transcript.
	Deepgram *integrations.DeepgramClient
}

func (t *YouTubeTransformer) Name() string { return "youtube" }

func (t *YouTubeTransformer) CanHandle(input *pipeline.RawInput) bool {
	u := input.URL
	return strings.Contains(u, "youtube.com/watch") ||
		strings.Contains(u, "youtu.be/") ||
		strings.Contains(u, "youtube.com/shorts/")
}

func (t *YouTubeTransformer) Transform(input *pipeline.RawInput) (*pipeline.TextDocument, error) {
	client := youtube.Client{}
	video, err := client.GetVideo(input.URL)
	if err != nil {
		return nil, fmt.Errorf("youtube: fetch video: %w", err)
	}

	title := video.Title
	if title == "" {
		title = "YouTube Video"
	}

	authors := []string{}
	if video.Author != "" {
		authors = append(authors, video.Author)
	}

	// 1) Try captions first.
	transcript := ""
	source := "youtube"
	if tracks := video.CaptionTracks; len(tracks) > 0 {
		track := pickEnglishTrack(tracks)
		raw, err := fetchCaptionTrack(track.BaseURL)
		if err == nil {
			transcript = parseTimedtextTranscript(raw)
		}
	}

	// 2) Fallback: download audio and transcribe via Deepgram.
	if transcript == "" && t.Deepgram != nil && t.Deepgram.Configured() {
		transcript, err = transcribeViaDeepgram(&client, video, t.Deepgram)
		if err == nil && transcript != "" {
			source = "youtube+deepgram"
		}
	}

	// 3) Last resort: description text.
	if transcript == "" {
		transcript = video.Description
	}
	if transcript == "" {
		return nil, fmt.Errorf("youtube: no captions, audio transcription, or description available for %s", input.URL)
	}

	duration := video.Duration.String()
	publishDate := video.PublishDate
	if publishDate.IsZero() {
		publishDate = time.Now()
	}

	return &pipeline.TextDocument{
		ID:      video.ID,
		Source:  source,
		Title:   title,
		URL:     input.URL,
		Date:    publishDate,
		Authors: authors,
		Content: transcript,
		Metadata: mergeMeta(input.Metadata, map[string]string{
			"url":      input.URL,
			"video_id": video.ID,
			"duration": duration,
			"channel":  video.Author,
		}),
	}, nil
}

// pickEnglishTrack returns the English caption track if one exists, otherwise
// the first track in the list.
func pickEnglishTrack(tracks []youtube.CaptionTrack) youtube.CaptionTrack {
	for _, t := range tracks {
		if strings.HasPrefix(strings.ToLower(t.LanguageCode), "en") {
			return t
		}
	}
	return tracks[0]
}

// fetchCaptionTrack downloads a caption URL (XML or VTT).
func fetchCaptionTrack(url string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("caption fetch http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// ytTranscriptXML matches YouTube's <transcript><text start=... dur=...>...</text></transcript>.
type ytTranscriptXML struct {
	Texts []ytTextLine `xml:"text"`
}
type ytTextLine struct {
	Start float64 `xml:"start,attr"`
	Text  string  `xml:",chardata"`
}

// parseTimedtextTranscript turns a YouTube timedtext XML payload into a
// formatted transcript with [HH:MM:SS] timestamps every few lines.
func parseTimedtextTranscript(payload string) string {
	var doc ytTranscriptXML
	if err := xml.Unmarshal([]byte(payload), &doc); err != nil || len(doc.Texts) == 0 {
		return ""
	}
	var b strings.Builder
	lastStamp := -60.0
	for _, line := range doc.Texts {
		text := decodeCommonEntities(strings.TrimSpace(line.Text))
		if text == "" {
			continue
		}
		if line.Start-lastStamp >= 30 {
			b.WriteString(fmt.Sprintf("\n[%s] ", formatHMS(line.Start)))
			lastStamp = line.Start
		}
		b.WriteString(text)
		b.WriteString(" ")
	}
	return strings.TrimSpace(b.String())
}

func formatHMS(seconds float64) string {
	total := int(seconds)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// transcribeViaDeepgram downloads the smallest audio stream and sends it
// to Deepgram. Used as a fallback when a video has no captions.
func transcribeViaDeepgram(client *youtube.Client, video *youtube.Video, dg *integrations.DeepgramClient) (string, error) {
	formats := video.Formats.Type("audio")
	if len(formats) == 0 {
		return "", fmt.Errorf("no audio streams available")
	}
	// Pick the smallest bitrate to minimize download size.
	sort.SliceStable(formats, func(i, j int) bool {
		return formats[i].Bitrate < formats[j].Bitrate
	})
	format := &formats[0]

	stream, _, err := client.GetStream(video, format)
	if err != nil {
		return "", fmt.Errorf("fetching audio stream: %w", err)
	}
	defer stream.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(stream, 200*1024*1024)); err != nil {
		return "", fmt.Errorf("downloading audio: %w", err)
	}

	mime := "audio/mp4"
	if strings.Contains(format.MimeType, "webm") {
		mime = "audio/webm"
	} else if strings.Contains(format.MimeType, "mp4") {
		mime = "audio/mp4"
	}

	result, err := dg.Transcribe(buf.Bytes(), mime)
	if err != nil {
		return "", err
	}
	return result.Transcript, nil
}
