/*

This file implements reading and writing the SubRip file format (*.srt).
It can parse *.srt files and create model from them.
And it also generate SubRip content from a model.

Format specifications:
https://en.wikipedia.org/wiki/SubRip
http://www.matroska.org/technical/specs/subtitles/srt.html

The parser is permissive, it tries to parse the input even if it does not conform to the specification.

Unofficial extensions are also supported and used.

Example of an SRT file:

    1
    00:02:17,440 --> 00:02:20,375
    Senator, we're making
    our final approach into Coruscant.

    2
    00:02:20,476 --> 00:02:22,501
    Very good, Lieutenant.

*/

package srtgears

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Mapping between *.srt pos to our model Pos for the {\anX} variant
var srtPosToModelPos = map[byte]Pos{
	'7': TopLeft, '8': Top, '9': TopRight,
	'4': Left, '5': Center, '6': Right,
	'1': BottomLeft, '2': Bottom, '3': BottomRight,
}

// Mapping between our model Pos to *.srt for the {\anX} variant
var modelPosToSrtPos = map[Pos]byte{
	TopLeft: '7', Top: '8', TopRight: '9',
	Left: '4', Center: '5', Right: '6',
	BottomLeft: '1', Bottom: '2', BottomRight: '3',
}

// ReadSrt reads and parses a SubRip stream (*.srt) and builds the model from it.
func ReadSrt(r io.Reader) (sp *SubsPack, err error) {
	sp = &SubsPack{}

	scanner := bufio.NewScanner(r)

	phase := 0

	var s *Subtitle

	addSub := func() {
		// Post process
		if len(s.Lines) > 0 {
			// find position spec in first line (e.g. {\anX})
			if line := s.Lines[0]; strings.HasPrefix(line, "{\a") {
				// 2 variants: {\anX} and {\aX}
				if len(line) >= 6 && line[3] == 'n' && line[5] == '}' {
					if p, ok := srtPosToModelPos[line[4]]; ok {
						s.Pos = p
						s.Lines[0] = line[6:] // Cut off pos spec from text
					}
				} else {
					// TODO
				}
			}
		}
		sp.Subs, s = append(sp.Subs, s), nil
	}

	for scanner.Scan() {
		line := scanner.Text()

		switch phase {
		case 0: // wanting sequence number, starting a new sub
			// discard seq#, we generate sequence numbres when writing
			s = &Subtitle{}
			phase++
		case 1: // wanting timestamps
			parseTimestamps(s, line)
			phase++
		case 2: // wanting subtitle lines
			if line == "" {
				// End of subtitle, separator
				addSub()
				phase = 0
			} else {
				s.Lines = append(s.Lines, line)
			}
		}
	}
	if s != nil { // Append last subtitle if there is no empty line at the end of input
		addSub()
	}

	err = scanner.Err()
	return
}

// Regexp pattern to extract data from timestamp lines.
// Very permissive, for example also accepts this line:
//     dY 00:02:20.476--->   00:02:22,501X Y
var timestampsPattern = regexp.MustCompile(`(\d\d):(\d\d):(\d\d)[,\.](\d\d\d)\s*-+>\s*(\d\d):(\d\d):(\d\d)[,\.](\d\d\d)`)

//                                            0 0 :  0 0 :  0 0  ,     0 0 0    -->     0 0 :  0 0 :  0 0  ,     0 0 0

// parseTimestamps parses a timestamp line
func parseTimestamps(s *Subtitle, line string) {
	// Example: 00:02:20,476 --> 00:02:22,501
	parts := timestampsPattern.FindStringSubmatch(line)
	if len(parts) == 0 {
		return // No match, invalid timestamp line
	}

	get := func(idx int) time.Duration {
		n, err := strconv.ParseInt(parts[idx], 10, 64)
		if err != nil {
			panic(err) // This shouldn't happen as only digits are matched.
		}
		return time.Duration(n)
	}

	// First part is the complete match
	s.TimeIn = time.Hour*get(1) + time.Minute*get(2) + time.Second*get(3) + time.Millisecond*get(4)
	s.TimeOut = time.Hour*get(5) + time.Minute*get(6) + time.Second*get(7) + time.Millisecond*get(8)
}

//WriteSrt generates SubRip format.
func WriteSrt(w io.Writer, s *SubsPack) (err error) {
	// noop writers: if there were a previous error, do nothing:
	pr := func(a ...interface{}) {
		if err == nil {
			_, err = fmt.Fprint(w, a...)
		}
	}
	prf := func(format string, a ...interface{}) {
		if err == nil {
			_, err = fmt.Fprintf(w, format, a...)
		}
	}

	newline := "\r\n" // Use Windows-style newline

	for i, v := range s.Subs {
		if err != nil {
			break
		}

		// Sequence number
		pr(i+1, newline)

		// Timestamps
		for tidx := 0; tidx < 2; tidx++ {
			var t time.Duration
			if tidx == 0 {
				t = v.TimeIn
			} else {
				t = v.TimeOut
			}
			hour := t / time.Hour
			min := (t % time.Hour) / time.Minute
			sec := (t % time.Minute) / time.Second
			ms := (t % time.Second) / time.Millisecond
			prf("%02d:%02d:%02d,%03d", hour, min, sec, ms)
			if tidx == 0 {
				pr(" --> ")
			} else {
				pr(newline)
			}
		}

		// Texts
		for i, line := range v.Lines {
			if i == 0 && v.Pos != PosNotSpecified {
				prf("{\an%c}", modelPosToSrtPos[v.Pos])
			}
			pr(line, newline)
		}

		// Separator: empty line
		pr(newline)
	}

	return
}
