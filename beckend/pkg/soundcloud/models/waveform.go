package models

// Waveform represents the sound wave amplitude samples for drawing the audio player visualization.
type Waveform struct {
	Width   int   `json:"width"`
	Height  int   `json:"height"`
	Samples []int `json:"samples"`
}
