package llm

import "github.com/cloudwego/eino/components/model"

// JSONSchemaResponseFormat describes a strict OpenAI structured-output
// request. It is provider-neutral at the service boundary; NewChatModel's
// adapter translates it to the installed Eino OpenAI implementation option.
type JSONSchemaResponseFormat struct {
	Name        string
	Description string
	Strict      bool
	Schema      map[string]any
}

// CallOptions are the LLM options owned by this application.
type CallOptions struct {
	ResponseFormat *JSONSchemaResponseFormat
}

// WithJSONSchemaResponse requests a response conforming to schema.
func WithJSONSchemaResponse(format JSONSchemaResponseFormat) model.Option {
	return model.WrapImplSpecificOptFn(func(options *CallOptions) {
		formatCopy := format
		options.ResponseFormat = &formatCopy
	})
}

// GetCallOptions decodes application-owned options. Chat model test doubles
// use this to observe the same contract translated by the real adapter.
func GetCallOptions(options ...model.Option) CallOptions {
	return *model.GetImplSpecificOptions(&CallOptions{}, options...)
}
