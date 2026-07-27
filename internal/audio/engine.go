package audio

// engine loops a WAV until it is stopped.
//
// How that is done differs enough between platforms to be worth an interface:
// Unix has command-line players that must be relaunched each time they reach
// the end, while Windows can loop natively inside this process.
type engine interface {
	// name identifies the mechanism, for the status line.
	name() string
	// start begins looping playback. Calling it while already playing does
	// nothing.
	start(path string) error
	// stop halts playback. Safe to call when nothing is playing, and safe to
	// call more than once.
	stop()
}
