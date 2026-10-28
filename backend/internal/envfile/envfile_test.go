package envfile

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHandlesCommentsBlankLinesQuotesAndExportPrefix(t *testing.T) {
	content := `# a comment

OPENAI_API_KEY=plain-key
OPENAI_BASE_URL="https://api.minimax.cn/v1"
export OPENAI_MODEL='MiniMax-M3.1-Flash-Preview'
SPACED=  trimmed value
EMPTY=
`
	values, err := Parse(bufio.NewReader(strings.NewReader(content)))
	require.NoError(t, err)

	assert.Equal(t, "plain-key", values["OPENAI_API_KEY"])
	assert.Equal(t, "https://api.minimax.cn/v1", values["OPENAI_BASE_URL"])
	assert.Equal(t, "MiniMax-M3.1-Flash-Preview", values["OPENAI_MODEL"])
	assert.Equal(t, "trimmed value", values["SPACED"])
	assert.Equal(t, "", values["EMPTY"])
}

func TestParseTrimsTrailingWhitespaceInsideValue(t *testing.T) {
	values, err := Parse(bufio.NewReader(strings.NewReader("OPENAI_BASE_URL=https://api.minimax.cn/v1 \n")))
	require.NoError(t, err)

	assert.Equal(t, "https://api.minimax.cn/v1", values["OPENAI_BASE_URL"])
}

func TestParseRejectsMalformedLine(t *testing.T) {
	_, err := Parse(bufio.NewReader(strings.NewReader("OPENAI_API_KEY\n")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected KEY=VALUE")
}

func TestLoadDoesNotOverrideExistingEnvironmentVariable(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "from-environment")

	directory := t.TempDir()
	path := filepath.Join(directory, ".env")
	require.NoError(t, os.WriteFile(path, []byte("OPENAI_API_KEY=from-file\nOPENAI_MODEL=from-file-model\n"), 0o600))

	require.NoError(t, Load(path))

	assert.Equal(t, "from-environment", os.Getenv("OPENAI_API_KEY"))
	assert.Equal(t, "from-file-model", os.Getenv("OPENAI_MODEL"))
}

func TestLoadIgnoresMissingFile(t *testing.T) {
	require.NoError(t, Load(filepath.Join(t.TempDir(), "absent")))
}

func TestLoadFromPathsUsesFirstExistingFile(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(second, ".env"), []byte("OPENAI_MODEL=second\n"), 0o600))
	require.NoError(t, os.Unsetenv("OPENAI_MODEL"))

	path, err := LoadFromPaths([]string{first, second})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(second, ".env"), path)
	assert.Equal(t, "second", os.Getenv("OPENAI_MODEL"))
}

func TestLoadFromPathsReturnsEmptyPathWhenNoFileExists(t *testing.T) {
	path, err := LoadFromPaths([]string{t.TempDir()})
	require.NoError(t, err)
	assert.Equal(t, "", path)
}
