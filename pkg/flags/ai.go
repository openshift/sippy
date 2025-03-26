package flags

import (
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/ai"
)

// AIFlags contains flags related to Sippy's use of generative AI.
type AIFlags struct {
	Endpoint string
	Model    string
}

func NewAIFlags() *AIFlags {
	return &AIFlags{}
}

func (f *AIFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.Endpoint, "ai-endpoint", "", "URL for an OpenAI-compatible endpoint. Set OPENAI_API_KEY to specify an API key.")
	fs.StringVar(&f.Model, "ai-model", "meta-llama/Llama-3.1-8B-Instruct", "The AI model to use for Sippy's generative AI")
}

func (f *AIFlags) GetLLMClient() *ai.LLMClient {
	return ai.NewLLMClient(f.Endpoint, f.Model)
}
