package llm

import (
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

var (
	codec     tokenizer.Codec
	codecOnce sync.Once
	codecErr  error
)

// getCodec returns the cl100k_base tokenizer (used by GPT-4, Claude, etc.)
func getCodec() (tokenizer.Codec, error) {
	codecOnce.Do(func() {
		codec, codecErr = tokenizer.Get(tokenizer.Cl100kBase)
	})
	return codec, codecErr
}

// EstimateTokens returns an approximate token count for the given text.
// Uses cl100k_base encoding which is a reasonable approximation for most models.
func EstimateTokens(text string) (int, error) {
	c, err := getCodec()
	if err != nil {
		return 0, err
	}

	ids, _, err := c.Encode(text)
	if err != nil {
		return 0, err
	}

	return len(ids), nil
}

// EstimateTokensSimple returns token count, defaulting to 0 on error.
func EstimateTokensSimple(text string) int {
	count, err := EstimateTokens(text)
	if err != nil {
		return 0
	}
	return count
}
