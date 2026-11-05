package speech

// VoiceService 是语音能力对 HTTP 层暴露的窄门面。
//
// 三个字段各自可缺省：Corrector 为 nil 时转写接口直接返回 ASR 原文，
// 不做错别字修复；Store / Transcriber 为 nil 时对应的 HTTP 端点返回 503。
type VoiceService struct {
	Store       *AudioStore
	Transcriber Transcriber
	Corrector   *Corrector
}

// NewVoiceService 组装门面。
func NewVoiceService(store *AudioStore, transcriber Transcriber, corrector *Corrector) *VoiceService {
	return &VoiceService{Store: store, Transcriber: transcriber, Corrector: corrector}
}
