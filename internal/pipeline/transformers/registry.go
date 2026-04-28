package transformers

import (
	"github.com/kriswong/corticalstack/internal/integrations"
	"github.com/kriswong/corticalstack/internal/pipeline"
	"github.com/kriswong/corticalstack/internal/vault"
)

// NewDefault returns all transformers in priority order.
// Order matters: the first CanHandle() match wins, so specific handlers
// (YouTube, LinkedIn, PDF, DOCX) come before generic webpage/html/passthrough.
//
// The vault is passed to transformers that preserve side-artifacts
// (DeepgramTransformer archives the original audio under
// vault/meetings/audio/ before transcribing).
func NewDefault(deepgram *integrations.DeepgramClient, v *vault.Vault) []pipeline.Transformer {
	return []pipeline.Transformer{
		&DeepgramTransformer{Client: deepgram, Vault: v},
		&VTTTransformer{},
		&PDFTransformer{},
		&DOCXTransformer{},
		&YouTubeTransformer{Deepgram: deepgram},
		&LinkedInTransformer{},
		&WebPageTransformer{},
		&HTMLTransformer{},
		&PassthroughTransformer{}, // always last — it accepts anything
	}
}
