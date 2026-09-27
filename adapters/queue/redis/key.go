package redis

func (a *redisAdapter) buildKey(topic, suffix string) string {
	buf := make([]byte, 0, len(a.prefix)+1+len(topic)+len(suffix))
	buf = append(buf, a.prefix...)
	buf = append(buf, ':')
	buf = append(buf, topic...)
	buf = append(buf, suffix...)
	return string(buf)
}

func (a *redisAdapter) readyKey(topic string) string      { return a.buildKey(topic, ":ready") }
func (a *redisAdapter) processingKey(topic string) string { return a.buildKey(topic, ":processing") }
func (a *redisAdapter) delayedKey(topic string) string    { return a.buildKey(topic, ":delayed") }
func (a *redisAdapter) deadlineKey(topic string) string {
	return a.buildKey(topic, ":processing_deadlines")
}
