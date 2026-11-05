// Package speech 提供 AI 模拟面试的语音能力：题目语音合成（TTS）、录音转写（ASR）
// 与转写文本的错别字修复。
package speech

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	// MaxAudioBytes 上传音频体积上限 20MB。
	MaxAudioBytes = 20 << 20
	// MaxAudioDuration 上传音频时长上限 10 分钟。
	MaxAudioDuration = 10 * time.Minute
)

// SynthesisRequest 是一次题目语音合成请求。
type SynthesisRequest struct {
	Text  string
	Voice string
}

// SynthesisResult 是合成产物。ContentType 恒为 audio/wav（不信上游 MIME，见 openai.go）。
type SynthesisResult struct {
	Audio       []byte
	ContentType string
}

// TranscriptionRequest 是一次录音转写请求；Audio 原样透传给上游，后端不转码。
type TranscriptionRequest struct {
	Audio       []byte
	ContentType string
}

// TranscriptionResult 是转写文本。
type TranscriptionResult struct {
	Text string
}

// Synthesizer 把题面文本合成为语音。
type Synthesizer interface {
	Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error)
}

// Transcriber 把录音转写为文本。
type Transcriber interface {
	Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResult, error)
}

// TextModel 是错别字修复所需的最小模型接口。
type TextModel interface {
	Generate(ctx context.Context, in []*schema.AgenticMessage, opts ...model.Option) (*schema.AgenticMessage, error)
}

// ValidateWAV 校验上传音频是可解析的 WAV，并返回时长。
// 只做校验，不转码：前端已保证 16kHz 单声道 PCM。
func ValidateWAV(data []byte) (time.Duration, error) {
	if len(data) > MaxAudioBytes {
		return 0, fmt.Errorf("speech: audio exceeds %d bytes", MaxAudioBytes)
	}
	if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return 0, errors.New("speech: not a RIFF/WAVE payload")
	}
	var channels, sampleRate, dataSize int
	for offset := 12; offset+8 <= len(data); {
		id := string(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		body := offset + 8
		switch id {
		case "fmt ":
			if body+16 <= len(data) {
				channels = int(binary.LittleEndian.Uint16(data[body+2 : body+4]))
				sampleRate = int(binary.LittleEndian.Uint32(data[body+4 : body+8]))
			}
		case "data":
			dataSize = size
		}
		offset = body + size
		if size%2 == 1 {
			offset++
		}
	}
	if channels <= 0 || sampleRate <= 0 || dataSize <= 0 {
		return 0, errors.New("speech: incomplete wav payload")
	}
	bytesPerSecond := sampleRate * channels * 2
	if bytesPerSecond <= 0 {
		return 0, errors.New("speech: invalid wav format")
	}
	duration := time.Duration(float64(dataSize) / float64(bytesPerSecond) * float64(time.Second))
	if duration > MaxAudioDuration {
		return 0, fmt.Errorf("speech: audio longer than %s", MaxAudioDuration)
	}
	return duration, nil
}
