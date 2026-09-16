package opusaudio

import (
	"encoding/binary"
	"io"
)

// Resampler linearly interpolates interleaved, 16-bit little-endian PCM
// from one sample rate to another, sample by sample, as an io.Reader.
type Resampler struct {
	src      io.Reader
	channels int
	ratio    float64

	initialized bool
	eof         bool
	cur, next   []int16
	fracPos     float64

	srcBuf []byte
	outBuf []byte
}

// NewResampler wraps src, whose PCM is at srcRate, so reads come back
// resampled to dstRate. If the rates already match, src is returned
// unchanged rather than wrapped.
func NewResampler(src io.Reader, srcRate, dstRate, channels int) io.Reader {
	if srcRate == dstRate {
		return src
	}

	return &Resampler{
		src:      src,
		channels: channels,
		ratio:    float64(srcRate) / float64(dstRate),
	}
}

// Read implements io.Reader.
func (r *Resampler) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		if len(r.outBuf) == 0 {
			sample, err := r.nextOutputSample()
			if err != nil {
				if n > 0 {
					return n, nil
				}
				return 0, err
			}
			r.outBuf = encodeSample(sample)
		}

		c := copy(p[n:], r.outBuf)
		n += c
		r.outBuf = r.outBuf[c:]
	}
	return n, nil
}

func (r *Resampler) nextOutputSample() ([]int16, error) {
	if r.eof {
		return nil, io.EOF
	}

	if !r.initialized {
		cur, err := r.readSourceSample()
		if err != nil {
			r.eof = true
			return nil, err
		}
		r.cur = cur

		next, err := r.readSourceSample()
		if err != nil {
			r.next = cur
		} else {
			r.next = next
		}
		r.initialized = true
	}

	out := make([]int16, r.channels)
	for ch := range out {
		out[ch] = int16(float64(r.cur[ch]) + (float64(r.next[ch])-float64(r.cur[ch]))*r.fracPos)
	}

	r.fracPos += r.ratio
	for r.fracPos >= 1.0 {
		r.fracPos -= 1.0
		r.cur = r.next

		next, err := r.readSourceSample()
		if err != nil {
			r.eof = true
			break
		}
		r.next = next
	}

	return out, nil
}

func (r *Resampler) readSourceSample() ([]int16, error) {
	frameBytes := r.channels * 2

	for len(r.srcBuf) < frameBytes {
		buf := make([]byte, frameBytes-len(r.srcBuf))
		m, err := r.src.Read(buf)
		r.srcBuf = append(r.srcBuf, buf[:m]...)
		if err != nil {
			if len(r.srcBuf) < frameBytes {
				return nil, err
			}
			break
		}
	}

	frame := r.srcBuf[:frameBytes]
	r.srcBuf = r.srcBuf[frameBytes:]

	samples := make([]int16, r.channels)
	for ch := 0; ch < r.channels; ch++ {
		samples[ch] = int16(binary.LittleEndian.Uint16(frame[ch*2:]))
	}
	return samples, nil
}

func encodeSample(samples []int16) []byte {
	out := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(s))
	}
	return out
}
