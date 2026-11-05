package speech

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestAudioStoreRoundTripIsolatesTenants(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)
	ctx := context.Background()

	if err := store.SaveQuestionAudio(ctx, 1, 10, 100, []byte("audio-a")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := store.SaveQuestionAudio(ctx, 2, 10, 100, []byte("audio-b")); err != nil {
		t.Fatalf("save other tenant: %v", err)
	}

	got, err := store.ReadQuestionAudio(1, 10, 100)
	if err != nil || string(got) != "audio-a" {
		t.Fatalf("read = %q err = %v", got, err)
	}
	other, err := store.ReadQuestionAudio(2, 10, 100)
	if err != nil || string(other) != "audio-b" {
		t.Fatalf("cross tenant read = %q err = %v", other, err)
	}

	expected := filepath.Join(root, "1", "10", "q100.wav")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected file at %s: %v", expected, err)
	}
}

func TestAudioStoreCleanupRemovesOnlyThatInterview(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)
	ctx := context.Background()

	_ = store.SaveQuestionAudio(ctx, 1, 10, 100, []byte("a"))
	_ = store.SaveQuestionAudio(ctx, 1, 11, 100, []byte("b"))

	if err := store.CleanupInterview(1, 10); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := store.ReadQuestionAudio(1, 10, 100); err == nil {
		t.Error("interview 10 audio should be removed")
	}
	if _, err := store.ReadQuestionAudio(1, 11, 100); err != nil {
		t.Errorf("interview 11 audio must survive: %v", err)
	}
}

// 音频不存在必须是可判定的错误：上层据此返回 404，而不是 500。
func TestAudioStoreReadMissingReportsErrAudioNotFound(t *testing.T) {
	store := NewAudioStore(t.TempDir())

	_, err := store.ReadQuestionAudio(1, 10, 100)
	if err == nil {
		t.Fatal("expected error for missing audio")
	}
	if !errors.Is(err, ErrAudioNotFound) {
		t.Errorf("err = %v, want ErrAudioNotFound", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want it to also match os.ErrNotExist", err)
	}
}

// 清理之后读取同样走「不存在」语义。
func TestAudioStoreReadAfterCleanupReportsErrAudioNotFound(t *testing.T) {
	store := NewAudioStore(t.TempDir())
	ctx := context.Background()
	if err := store.SaveQuestionAudio(ctx, 1, 10, 100, []byte("a")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := store.CleanupInterview(1, 10); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := store.ReadQuestionAudio(1, 10, 100); !errors.Is(err, ErrAudioNotFound) {
		t.Errorf("err = %v, want ErrAudioNotFound", err)
	}
}

// ID 是外部输入：非正数一律拒绝，避免出现 "-1" 这类目录名。
func TestAudioStoreRejectsNonPositiveIDs(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)
	ctx := context.Background()

	if err := store.SaveQuestionAudio(ctx, 0, 10, 100, []byte("a")); err == nil {
		t.Error("tenant id 0 should be rejected")
	}
	if err := store.SaveQuestionAudio(ctx, 1, -1, 100, []byte("a")); err == nil {
		t.Error("negative interview id should be rejected")
	}
	if err := store.SaveQuestionAudio(ctx, 1, 10, -5, []byte("a")); err == nil {
		t.Error("negative question id should be rejected")
	}
	if _, err := store.ReadQuestionAudio(1, 10, 0); err == nil {
		t.Error("question id 0 should be rejected")
	}
	if err := store.CleanupInterview(1, 0); err == nil {
		t.Error("interview id 0 should be rejected")
	}
	// 被拒绝的调用不能留下任何目录。
	if _, err := os.Stat(filepath.Join(root, "-1")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("unexpected directory created: %v", err)
	}
}

// 空音频写盘没有意义，直接拒绝。
func TestAudioStoreRejectsEmptyAudio(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)

	if err := store.SaveQuestionAudio(context.Background(), 1, 10, 100, nil); err == nil {
		t.Error("empty audio should be rejected")
	}
	if _, err := os.Stat(filepath.Join(root, "1", "10", "q100.wav")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("empty audio must not be written: %v", err)
	}
}

// Context 取消要能立刻中止，不落盘。
func TestAudioStoreRespectsCanceledContext(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.SaveQuestionAudio(ctx, 1, 10, 100, []byte("a")); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(filepath.Join(root, "1")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("canceled save must not touch disk: %v", err)
	}
}

// 保存成功后目录里只应该有最终文件，不能留临时文件。
func TestAudioStoreLeavesNoTemporaryFileAfterSave(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)

	if err := store.SaveQuestionAudio(context.Background(), 1, 10, 100, []byte("a")); err != nil {
		t.Fatalf("save: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "1", "10"))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "q100.wav" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir entries = %v, want only q100.wav", names)
	}
}

// 提交阶段失败时临时文件要被清掉，不能留下半截文件。
func TestAudioStoreRemovesTemporaryFileWhenCommitFails(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)
	dir := filepath.Join(root, "1", "10")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// 让最终路径被一个目录占住，rename 必然失败。
	if err := os.MkdirAll(filepath.Join(dir, "q100.wav"), 0o755); err != nil {
		t.Fatalf("mkdir blocker: %v", err)
	}

	if err := store.SaveQuestionAudio(context.Background(), 1, 10, 100, []byte("a")); err == nil {
		t.Fatal("expected commit error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
}

// 同一题被并发写入：最终文件必须是某一次写入的完整内容，不能是两份内容拼在一起。
func TestAudioStoreConcurrentSaveKeepsFileIntact(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)
	ctx := context.Background()

	payloads := [][]byte{
		[]byte("aaaaaaaa"),
		[]byte("bbbbbbbb"),
		[]byte("cccccccc"),
		[]byte("dddddddd"),
	}
	var wg sync.WaitGroup
	for _, payload := range payloads {
		payload := payload
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := store.SaveQuestionAudio(ctx, 1, 10, 100, payload); err != nil {
				t.Errorf("save: %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := store.ReadQuestionAudio(1, 10, 100)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	valid := false
	for _, payload := range payloads {
		if string(got) == string(payload) {
			valid = true
			break
		}
	}
	if !valid {
		t.Errorf("content = %q, want one of the written payloads", got)
	}
	entries, err := os.ReadDir(filepath.Join(root, "1", "10"))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("dir entries = %d, want 1", len(entries))
	}
}

// 清理单元是 interviewID：同租户另一场面试、以及另一租户的同 ID 面试都不受影响。
func TestAudioStoreCleanupDoesNotTouchOtherTenants(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)
	ctx := context.Background()

	if err := store.SaveQuestionAudio(ctx, 1, 10, 100, []byte("a")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := store.SaveQuestionAudio(ctx, 2, 10, 100, []byte("b")); err != nil {
		t.Fatalf("save other tenant: %v", err)
	}
	if err := store.CleanupInterview(1, 10); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := store.ReadQuestionAudio(2, 10, 100); err != nil {
		t.Errorf("other tenant audio must survive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "2", "10", "q100.wav")); err != nil {
		t.Errorf("other tenant file must survive: %v", err)
	}
}

// 清理要顺带删掉遗留的临时文件。
func TestAudioStoreCleanupRemovesLeftoverTemporaryFiles(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)
	ctx := context.Background()
	if err := store.SaveQuestionAudio(ctx, 1, 10, 100, []byte("a")); err != nil {
		t.Fatalf("save: %v", err)
	}
	leftover := filepath.Join(root, "1", "10", "q100.wav.tmp-999")
	if err := os.WriteFile(leftover, []byte("junk"), 0o600); err != nil {
		t.Fatalf("write leftover: %v", err)
	}
	if err := store.CleanupInterview(1, 10); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "1", "10")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("interview dir should be gone: %v", err)
	}
}

// 清理不存在的面试必须幂等，不能报错。
func TestAudioStoreCleanupIsIdempotent(t *testing.T) {
	store := NewAudioStore(t.TempDir())
	if err := store.CleanupInterview(1, 10); err != nil {
		t.Fatalf("cleanup of missing interview should succeed: %v", err)
	}
}

// Root 返回配置的根目录（启动日志用）。
func TestAudioStoreRootReturnsConfiguredDirectory(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)
	if store.Root() != root {
		t.Errorf("root = %q, want %q", store.Root(), root)
	}
}

// 面试目录被换成软链时，写/读/清理都必须拒绝，不能把数据写到 root 之外。
func TestAudioStoreRejectsSymlinkedInterviewDir(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	store := NewAudioStore(root)
	ctx := context.Background()

	if err := os.MkdirAll(filepath.Join(root, "1"), 0o755); err != nil {
		t.Fatalf("mkdir tenant dir: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "1", "10")); err != nil {
		t.Fatalf("symlink interview dir: %v", err)
	}

	if err := store.SaveQuestionAudio(ctx, 1, 10, 100, []byte("a")); err == nil {
		t.Error("save through symlinked interview dir should be rejected")
	}
	if _, err := os.Stat(filepath.Join(outside, "q100.wav")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("audio must not be written outside root: %v", err)
	}
	if _, err := store.ReadQuestionAudio(1, 10, 100); err == nil {
		t.Error("read through symlinked interview dir should be rejected")
	}
	if err := store.CleanupInterview(1, 10); err == nil {
		t.Error("cleanup through symlinked interview dir should be rejected")
	}
	// 外部目录不能被清理掉。
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("outside dir must survive: %v", err)
	}
}

// 租户目录被换成软链时同样必须拒绝。
func TestAudioStoreRejectsSymlinkedTenantDir(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	store := NewAudioStore(root)
	ctx := context.Background()

	if err := os.Symlink(outside, filepath.Join(root, "1")); err != nil {
		t.Fatalf("symlink tenant dir: %v", err)
	}

	if err := store.SaveQuestionAudio(ctx, 1, 10, 100, []byte("a")); err == nil {
		t.Error("save through symlinked tenant dir should be rejected")
	}
	if _, err := os.Stat(filepath.Join(outside, "10", "q100.wav")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("audio must not be written outside root: %v", err)
	}
	if _, err := store.ReadQuestionAudio(1, 10, 100); err == nil {
		t.Error("read through symlinked tenant dir should be rejected")
	}
}

// 音频文件本身是软链时读取必须拒绝：否则会把 root 之外的内容读出来。
func TestAudioStoreRejectsSymlinkedAudioFile(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.wav")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	store := NewAudioStore(root)
	dir := filepath.Join(root, "1", "10")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir interview dir: %v", err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, "q100.wav")); err != nil {
		t.Fatalf("symlink audio file: %v", err)
	}

	data, err := store.ReadQuestionAudio(1, 10, 100)
	if err == nil {
		t.Fatalf("read through symlinked audio file should be rejected, got %q", data)
	}
	if len(data) != 0 {
		t.Errorf("no content should be returned, got %q", data)
	}
	// 软链不是「文件不存在」：不能被上层当成 404。
	if errors.Is(err, ErrAudioNotFound) {
		t.Errorf("symlink must not be reported as ErrAudioNotFound: %v", err)
	}
}

// 写入时 rename 会把软链本身替换掉，而不是改写软链指向的文件。
func TestAudioStoreSaveReplacesSymlinkedAudioFile(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.wav")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	store := NewAudioStore(root)
	dir := filepath.Join(root, "1", "10")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir interview dir: %v", err)
	}
	target := filepath.Join(dir, "q100.wav")
	if err := os.Symlink(secret, target); err != nil {
		t.Fatalf("symlink audio file: %v", err)
	}

	if err := store.SaveQuestionAudio(context.Background(), 1, 10, 100, []byte("new")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if content, err := os.ReadFile(secret); err != nil || string(content) != "secret" {
		t.Errorf("external file must not be modified: content=%q err=%v", content, err)
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("lstat audio file: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("audio file should be a regular file after save")
	}
}

// 存储层必须有体积上限：异常的大响应不能把磁盘撑爆。
func TestAudioStoreRejectsOversizedAudio(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)
	oversized := make([]byte, MaxAudioBytes+1)

	err := store.SaveQuestionAudio(context.Background(), 1, 10, 100, oversized)
	if err == nil {
		t.Fatal("oversized audio should be rejected")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("err = %v, want a too-large error", err)
	}
	if _, err := os.Stat(filepath.Join(root, "1", "10", "q100.wav")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("oversized audio must not be written: %v", err)
	}
}

// 刚好等于上限的音频要能写进去（边界不误伤）。
func TestAudioStoreAcceptsAudioAtSizeLimit(t *testing.T) {
	root := t.TempDir()
	store := NewAudioStore(root)
	atLimit := make([]byte, MaxAudioBytes)

	if err := store.SaveQuestionAudio(context.Background(), 1, 10, 100, atLimit); err != nil {
		t.Fatalf("save at size limit: %v", err)
	}
	got, err := store.ReadQuestionAudio(1, 10, 100)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != MaxAudioBytes {
		t.Errorf("len = %d, want %d", len(got), MaxAudioBytes)
	}
}

// Root 返回绝对化后的目录：相对路径构造时也必须是绝对路径。
func TestAudioStoreRootIsAbsoluteForRelativeRoot(t *testing.T) {
	t.Chdir(t.TempDir())
	store := NewAudioStore("rel-audio-root")

	root := store.Root()
	if !filepath.IsAbs(root) {
		t.Fatalf("root = %q, want an absolute path", root)
	}
	if root == "rel-audio-root" {
		t.Fatalf("root = %q, want it to be absolutized", root)
	}
	// 落盘位置跟随绝对化后的 root。
	if err := store.SaveQuestionAudio(context.Background(), 1, 10, 100, []byte("a")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "1", "10", "q100.wav")); err != nil {
		t.Errorf("expected audio under %s: %v", root, err)
	}
}
