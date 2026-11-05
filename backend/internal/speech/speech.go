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

// wavAudioFormatPCM 是 WAV fmt 块里的 PCM 编码标识。
const wavAudioFormatPCM = 1

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
//
// 校验项：RIFF/WAVE magic、fmt 块的 PCM 编码与声道/采样率/位深、data 块实际长度是否完整。
// 时长按 fmt 块声明的字节率（采样率 × 声道 × 位深/8）估算，因此 8bit/16bit/24bit 都能算对。
func ValidateWAV(data []byte) (time.Duration, error) {
	if len(data) > MaxAudioBytes {
		return 0, fmt.Errorf("speech: audio exceeds %d bytes", MaxAudioBytes)
	}
	if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return 0, errors.New("speech: not a RIFF/WAVE payload")
	}
	var channels, sampleRate, bitsPerSample, dataSize int
	// 用 int64 走 chunk 偏移：chunk 长度来自文件头（uint32），32 位平台下转 int 会溢出。
	for offset := int64(12); offset+8 <= int64(len(data)); {
		id := string(data[offset : offset+4])
		size := int64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		body := offset + 8
		switch id {
		case "fmt ":
			// fmt 块至少 16 字节（PCM）；长度不足时按「不完整」处理，留给后面的兜底判断报错。
			if size >= 16 && body+16 <= int64(len(data)) {
				audioFormat := int(binary.LittleEndian.Uint16(data[body : body+2]))
				if audioFormat != wavAudioFormatPCM {
					return 0, errors.New("speech: only pcm wav is supported")
				}
				channels = int(binary.LittleEndian.Uint16(data[body+2 : body+4]))
				sampleRate = int(binary.LittleEndian.Uint32(data[body+4 : body+8]))
				bitsPerSample = int(binary.LittleEndian.Uint16(data[body+14 : body+16]))
			}
		case "data":
			// data 块声明长度必须真的在文件里，否则是截断音频（时长也会是假的）。
			// 长度未知的流式 WAV（data size = 0xFFFFFFFF）同样落在这里被拒绝：
			// 前端产出的是完整 PCM WAV，不需要支持流式变体。
			if size > int64(len(data))-body {
				return 0, errors.New("speech: truncated wav data chunk")
			}
			// WAV 规范只允许一个 data 块；万一出现多个，取最大的那个——
			// 比「取最后一个」更接近真实音频量（最后一个可能只是附加块）。
			if int(size) > dataSize {
				dataSize = int(size)
			}
		}
		offset = body + size
		if size%2 == 1 {
			offset++
		}
	}
	if channels <= 0 || sampleRate <= 0 || dataSize <= 0 {
		return 0, errors.New("speech: incomplete wav payload")
	}
	if bitsPerSample <= 0 || bitsPerSample%8 != 0 {
		return 0, errors.New("speech: invalid wav format")
	}
	// data 块必须按帧对齐：至少一整帧，且不能是半帧，否则算出来的时长是编造的。
	blockAlign := channels * bitsPerSample / 8
	if blockAlign <= 0 || dataSize < blockAlign || dataSize%blockAlign != 0 {
		return 0, errors.New("speech: invalid wav format")
	}
	bytesPerSecond := int64(sampleRate) * int64(channels) * int64(bitsPerSample/8)
	if bytesPerSecond <= 0 {
		return 0, errors.New("speech: invalid wav format")
	}
	duration := time.Duration(float64(dataSize) / float64(bytesPerSecond) * float64(time.Second))
	if duration > MaxAudioDuration {
		return 0, fmt.Errorf("speech: audio longer than %s", MaxAudioDuration)
	}
	return duration, nil
}
