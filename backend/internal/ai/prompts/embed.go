package prompts

import "embed"

//go:embed *.tmpl
var FS embed.FS

func MustRead(name string) string {
	b, err := FS.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return string(b)
}
