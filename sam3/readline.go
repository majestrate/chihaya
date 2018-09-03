package sam3

import "io"

func readLine(r io.Reader) (line string, err error) {
	var buff [1]byte
	for err == nil {
		_, err = r.Read(buff[:])
		if err == nil {
			if buff[0] == 10 {
				return line, err
			} else {
				line += string(buff[:])
			}
		}
	}
	return

}
