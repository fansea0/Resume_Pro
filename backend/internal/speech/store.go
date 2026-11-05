package speech

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// audioDirPerm 是音频目录权限，与 files/local.go 的本地存储保持一致。
	audioDirPerm = 0o755
	// audioFilePerm 是音频文件权限：题目语音属于租户数据，只允许属主读写。
	audioFilePerm = 0o600
)

// ErrAudioNotFound 表示题目没有已生成的音频。
//
// 上层（HTTP 层）据此返回 404 而不是 500。它同时包住 os.ErrNotExist，
// 因此 errors.Is(err, os.ErrNotExist) 也能判定为「文件不存在」。
var ErrAudioNotFound = fmt.Errorf("speech: question audio not found: %w", os.ErrNotExist)

// AudioStore 以 {tenantID}/{interviewID}/q{questionID}.wav 组织题目音频。
//
// 清理单元是 interviewID：面试结束 = 删一个目录，不需要跨目录反查引用。
// 不同租户的同一 interviewID 落在不同目录下，互不干扰。
type AudioStore struct {
	root string
}

// NewAudioStore 创建存储。root 不存在时在首次写入时创建。
//
// root 会被转成绝对路径：配置里给相对路径时，落盘位置不应随进程工作目录漂移。
func NewAudioStore(root string) *AudioStore {
	if strings.TrimSpace(root) == "" {
		return &AudioStore{}
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return &AudioStore{root: root}
	}
	return &AudioStore{root: absolute}
}

// Root 返回音频根目录（启动日志用）。
func (s *AudioStore) Root() string {
	return s.root
}

// interviewDir 返回一场面试的音频目录。
func (s *AudioStore) interviewDir(tenantID, interviewID int64) (string, error) {
	if err := validateAudioID("tenant", tenantID); err != nil {
		return "", err
	}
	if err := validateAudioID("interview", interviewID); err != nil {
		return "", err
	}
	dir := filepath.Join(s.root, strconv.FormatInt(tenantID, 10), strconv.FormatInt(interviewID, 10))
	return s.ensureUnderRoot(dir)
}

// questionPath 返回一道题目的音频文件路径。
func (s *AudioStore) questionPath(tenantID, interviewID, questionID int64) (string, error) {
	if err := validateAudioID("question", questionID); err != nil {
		return "", err
	}
	dir, err := s.interviewDir(tenantID, interviewID)
	if err != nil {
		return "", err
	}
	return s.ensureUnderRoot(filepath.Join(dir, fmt.Sprintf("q%d.wav", questionID)))
}

// ensureUnderRoot 防御性检查：拼出来的路径必须落在 root 之下。
//
// ID 已经被校验成正整数，路径逃逸在结构上不可能发生；这里只是把「不可能」变成「一旦发生就报错」。
func (s *AudioStore) ensureUnderRoot(path string) (string, error) {
	relative, err := filepath.Rel(s.root, path)
	if err != nil {
		return "", fmt.Errorf("speech: audio path escapes root: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("speech: audio path escapes root: %s", path)
	}
	return path, nil
}

// validateAudioID 校验 ID 必须为正数。
//
// ID 来自数据库自增主键，非正数一定是上游传错，越早报错越好；
// 同时保证目录名里不会出现 "-" 之类的字符。
func validateAudioID(kind string, id int64) error {
	if id <= 0 {
		return fmt.Errorf("speech: invalid %s id: %d", kind, id)
	}
	return nil
}

// SaveQuestionAudio 落盘题目语音。
//
// 先写同目录下的临时文件再 rename：生成 worker 与读取方并发时不会读到半个文件。
// 临时名带唯一后缀，同一题被并发写入时不会互相覆盖临时文件。
//
// 只校验「非空」：音频格式由调用方保证（TTS 侧由 provider 契约保证，
// 上传转写侧由 ValidateWAV 保证），存储层不做格式判断。
func (s *AudioStore) SaveQuestionAudio(ctx context.Context, tenantID, interviewID, questionID int64, audio []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.root == "" {
		return errors.New("speech: audio root is required")
	}
	if len(audio) == 0 {
		return errors.New("speech: empty question audio")
	}
	path, err := s.questionPath(tenantID, interviewID, questionID)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, audioDirPerm); err != nil {
		return fmt.Errorf("speech: create audio dir: %w", err)
	}

	// 临时文件与最终文件同目录，保证 rename 不跨文件系统（跨文件系统的 rename 不是原子的）。
	temporary, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("speech: create audio temp file: %w", err)
	}
	temporaryPath := temporary.Name()
	// 失败路径统一删除临时文件：残留的 .tmp 不会被读取方当成音频，但会一直占空间。
	defer func() { _ = os.Remove(temporaryPath) }()

	if _, err := temporary.Write(audio); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("speech: write audio: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("speech: close audio temp file: %w", err)
	}
	if err := os.Chmod(temporaryPath, audioFilePerm); err != nil {
		return fmt.Errorf("speech: chmod audio file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("speech: commit audio: %w", err)
	}
	return nil
}

// ReadQuestionAudio 读取题目语音字节。
//
// 文件不存在时返回包住 ErrAudioNotFound 的错误：上层据此返回 404，
// 其他错误（权限、IO 故障）才按 500 处理。
func (s *AudioStore) ReadQuestionAudio(tenantID, interviewID, questionID int64) ([]byte, error) {
	if s.root == "" {
		return nil, errors.New("speech: audio root is required")
	}
	path, err := s.questionPath(tenantID, interviewID, questionID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("speech: read question audio %d: %w", questionID, ErrAudioNotFound)
		}
		return nil, fmt.Errorf("speech: read question audio %d: %w", questionID, err)
	}
	return data, nil
}

// CleanupInterview 删除一场面试的全部音频目录（含遗留的临时文件）。
//
// 目录不存在时返回 nil：清理是幂等的，面试终态清理可以放心重复调用。
func (s *AudioStore) CleanupInterview(tenantID, interviewID int64) error {
	if s.root == "" {
		return errors.New("speech: audio root is required")
	}
	dir, err := s.interviewDir(tenantID, interviewID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("speech: cleanup interview audio: %w", err)
	}
	return nil
}
