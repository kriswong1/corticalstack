package transformers

import (
	"github.com/kriswong/corticalstack/internal/integrations"
	"github.com/kriswong/corticalstack/internal/pipeline"
)

// NewDefault returns all transformers in priority order.
// Order matters: the first CanHandle() match wins, so specific handlers
// (YouTube, LinkedIn, PDF, DOCX) come before generic webpage/html/passthrough.
func NewDefault(deepgram *integrations.DeepgramClient) []pipeline.Transformer {
	return []pipeline.Transformer{
		&DeepgramTransformer{Client: deepgram},
		&PDFTransformer{},
		&DOCXTransformer{},
		&YouTubeTransformer{Deepgram: deepgram},
		&LinkedInTransformer{},
		&WebPageTransformer{},
		&HTMLTransformer{},
		&PassthroughTransformer{}, // always last — it accepts anything
	}
}
