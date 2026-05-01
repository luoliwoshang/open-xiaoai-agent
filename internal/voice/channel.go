package voice

import "time"

// Channel 表示一个可用于语音播报的输出通道。
//
// assistant 主流程只关心两件事：
// 1. 开始播报前，这个通道是否需要做额外准备；
// 2. 给它一段完整文本后，它能不能把这段话说出来。
//
// 当前小爱只是 Channel 的一个实现；后续也可以接入别的语音端。
type Channel interface {
	PreparePlayback(opts PlaybackOptions) error
	SpeakText(text string, timeout time.Duration) error
}

// PlaybackOptions 描述一轮正式播报开始前的准备策略。
//
// 这里不把“小爱 abort”暴露给主流程，而是统一抽象成：
// - 是否需要抢占/中断原生链路；
// - 这次调用前是否已经做过一次抢占；
// - 抢占后是否需要等待一小段时间再开始播报。
type PlaybackOptions struct {
	InterruptNativeFlow   bool
	NativeFlowInterrupted bool
	PostInterruptDelay    time.Duration
}
