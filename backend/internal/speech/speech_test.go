package speech

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

// wavSpec 描述 WAV 文件 fmt 块里的格式信息。
type wavSpec struct {
	audioFormat   uint16
	channels      uint16
	sampleRate    uint32
	bitsPerSample uint16
}

// buildWAV 构造一个 WAV 文件（fmt 块 + data 块）。
// dataBytes 是实际写入的字节数，declaredDataSize 是 data 块头里声明的长度；
// 两者不同即可构造出「声明长度大于实际长度」的截断文件。
func buildWAV(spec wavSpec, dataBytes int, declaredDataSize int) []byte {
	blockAlign := spec.channels * spec.bitsPerSample / 8
	byteRate := spec.sampleRate * uint32(blockAlign)

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+declaredDataSize))
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, spec.audioFormat)
	_ = binary.Write(&buf, binary.LittleEndian, spec.channels)
	_ = binary.Write(&buf, binary.LittleEndian, spec.sampleRate)
	_ = binary.Write(&buf, binary.LittleEndian, byteRate)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&buf, binary.LittleEndian, spec.bitsPerSample)

	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(declaredDataSize))
	buf.Write(make([]byte, dataBytes))
	return buf.Bytes()
}

// buildWAVWithDataChunks 构造带多个 data 块的 WAV（验证多块时的取值策略）。
func buildWAVWithDataChunks(spec wavSpec, chunks []int) []byte {
	// buildWAV(spec, 0, 0) 产出 44 字节头部（末尾含一个空 data 块头），
	// 去掉末尾 8 字节后由下面自己写多个 data 块。
	base := buildWAV(spec, 0, 0)
	base = base[:len(base)-8]
	for _, size := range chunks {
		base = append(base, []byte("data")...)
		var header [4]byte
		binary.LittleEndian.PutUint32(header[:], uint32(size))
		base = append(base, header[:]...)
		base = append(base, make([]byte, size)...)
		if size%2 == 1 {
			base = append(base, 0)
		}
	}
	return base
}

func pcm16Mono16k() wavSpec {
	return wavSpec{audioFormat: 1, channels: 1, sampleRate: 16000, bitsPerSample: 16}
}

func assertDurationClose(t *testing.T, got, want time.Duration) {
	t.Helper()
	if diff := got - want; diff > 5*time.Millisecond || diff < -5*time.Millisecond {
		t.Errorf("duration = %s, want %s (±5ms)", got, want)
	}
}

func TestValidateWAVReturnsDuration(t *testing.T) {
	cases := []struct {
		name string
		spec wavSpec
		data int
		want time.Duration
	}{
		{name: "16bit 单声道 16kHz", spec: pcm16Mono16k(), data: 32000, want: time.Second},
		{
			name: "16bit 立体声 48kHz",
			spec: wavSpec{audioFormat: 1, channels: 2, sampleRate: 48000, bitsPerSample: 16},
			data: 192000,
			want: time.Second,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			duration, err := ValidateWAV(buildWAV(tc.spec, tc.data, tc.data))
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			assertDurationClose(t, duration, tc.want)
		})
	}
}

func TestValidateWAVRejectsTruncatedDataChunk(t *testing.T) {
	// 头里声明 16000 字节 data，实际一个字节都没有。
	_, err := ValidateWAV(buildWAV(pcm16Mono16k(), 0, 16000))
	if err == nil {
		t.Fatal("expected error for truncated data chunk")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("error = %v, want truncated data chunk error", err)
	}
}

func TestValidateWAVSupportsNon16Bit(t *testing.T) {
	cases := []struct {
		name string
		spec wavSpec
		data int
	}{
		{
			name: "8bit 单声道 16kHz",
			spec: wavSpec{audioFormat: 1, channels: 1, sampleRate: 16000, bitsPerSample: 8},
			data: 16000,
		},
		{
			name: "24bit 单声道 16kHz",
			spec: wavSpec{audioFormat: 1, channels: 1, sampleRate: 16000, bitsPerSample: 24},
			data: 48000,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			duration, err := ValidateWAV(buildWAV(tc.spec, tc.data, tc.data))
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			assertDurationClose(t, duration, time.Second)
		})
	}
}

func TestValidateWAVRejectsLongAudio(t *testing.T) {
	// 16bit 16kHz 单声道：19232000 字节 = 601 秒，超过 10 分钟上限；
	// 体积仍低于 20MB 上限，因此只会被时长卡住。
	long := buildWAV(pcm16Mono16k(), 19232000, 19232000)
	_, err := ValidateWAV(long)
	if err == nil {
		t.Fatal("expected error for audio longer than limit")
	}
	if !strings.Contains(err.Error(), "longer than") {
		t.Errorf("error = %v, want duration limit error", err)
	}
}

// data 块必须按帧对齐：半帧数据不能被当成有效音频。
func TestValidateWAVRejectsUnalignedDataChunk(t *testing.T) {
	cases := []struct {
		name string
		data int
	}{
		{name: "1 字节（不足一帧）", data: 1},
		{name: "3 字节（半帧）", data: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateWAV(buildWAV(pcm16Mono16k(), tc.data, tc.data))
			if err == nil {
				t.Fatal("expected error for unaligned data chunk")
			}
			if !strings.Contains(err.Error(), "invalid wav format") {
				t.Errorf("error = %v, want invalid wav format error", err)
			}
		})
	}
}

// 多个 data 块时按最大的那个算时长（与真实音频量一致）。
func TestValidateWAVUsesLargestDataChunk(t *testing.T) {
	cases := []struct {
		name   string
		chunks []int
	}{
		{name: "大块在前", chunks: []int{32000, 16000}},
		{name: "大块在后", chunks: []int{16000, 32000}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			duration, err := ValidateWAV(buildWAVWithDataChunks(pcm16Mono16k(), tc.chunks))
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			assertDurationClose(t, duration, time.Second)
		})
	}
}

func TestValidateWAVRejectsNonPCM(t *testing.T) {
	spec := wavSpec{audioFormat: 3, channels: 1, sampleRate: 16000, bitsPerSample: 32}
	_, err := ValidateWAV(buildWAV(spec, 64000, 64000))
	if err == nil {
		t.Fatal("expected error for non-pcm wav")
	}
	if !strings.Contains(err.Error(), "pcm") {
		t.Errorf("error = %v, want pcm format error", err)
	}
}

func TestValidateWAVRejectsBadMagic(t *testing.T) {
	data := buildWAV(pcm16Mono16k(), 32000, 32000)
	data[0] = 'X'
	_, err := ValidateWAV(data)
	if err == nil {
		t.Fatal("expected error for bad magic")
	}
	if !strings.Contains(err.Error(), "RIFF") {
		t.Errorf("error = %v, want riff magic error", err)
	}
}

func TestValidateWAVRejectsOversizePayload(t *testing.T) {
	data := make([]byte, MaxAudioBytes+1)
	copy(data, buildWAV(pcm16Mono16k(), 0, 0))
	_, err := ValidateWAV(data)
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %v, want size limit error", err)
	}
}
