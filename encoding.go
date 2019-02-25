package pmmime

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"regexp"
	"strings"

	"encoding/base64"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
)

var wordDec = &mime.WordDecoder{
	CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
		dec, err := selectDecoder(charset)
		if err != nil {
			return nil, err
		}
		if dec == nil { // utf-8
			return input, nil
		}
		return dec.Reader(input), nil
	},
}

// expected trimmed low case
func getEncoding(charset string) (enc encoding.Encoding, err error) {
	preparsed := charset

	// koi
	re := regexp.MustCompile("(cs)?koi[-_ ]?8?[-_ ]?(r|ru|u|uk)?$")
	matches := re.FindAllStringSubmatch(charset, -1)
	if len(matches) == 1 && len(matches[0]) == 3 {
		preparsed = "koi8-"
		switch matches[0][2] {
		case "u", "uk":
			preparsed += "u"
		default:
			preparsed += "r"
		}
	}

	// windows-XXXX
	re = regexp.MustCompile("(cp|(cs)?win(dows)?)[-_ ]?([0-9]{3,4})$")
	matches = re.FindAllStringSubmatch(charset, -1)
	if len(matches) == 1 && len(matches[0]) == 5 {
		switch matches[0][4] {
		case "874", "1250", "1251", "1252", "1253", "1254", "1255", "1256", "1257", "1258":
			preparsed = "windows-" + matches[0][4]
		}
	}

	// iso
	re = regexp.MustCompile("iso[-_ ]?([0-9]{4})[-_ ]?([0-9]+|jp)?[-_ ]?(i|e)?")
	matches = re.FindAllStringSubmatch(charset, -1)
	if len(matches) == 1 && len(matches[0]) == 4 {
		if matches[0][1] == "2022" && matches[0][2] == "jp" {
			preparsed = "iso-2022-jp"
		}
		if matches[0][1] == "8859" {
			switch matches[0][2] {
			case "1", "2", "3", "4", "5", "7", "8", "9", "10", "11", "13", "14", "15", "16":
				preparsed = "iso-8859-" + matches[0][2]
				if matches[0][3] == "i" {
					preparsed += "-" + matches[0][3]
				}
			case "":
				preparsed = "iso-8859-1"
			}
		}
	}

	// latin is tricky
	re = regexp.MustCompile("^(cs|csiso)?l(atin)?[-_ ]?([0-9]{1,2})$")
	matches = re.FindAllStringSubmatch(charset, -1)
	if len(matches) == 1 && len(matches[0]) == 4 {
		switch matches[0][3] {
		case "1":
			preparsed = "windows-1252"
		case "2", "3", "4", "5":
			preparsed = "iso-8859-" + matches[0][3]
		case "6":
			preparsed = "iso-8859-10"
		case "8":
			preparsed = "iso-8859-14"
		case "9":
			preparsed = "iso-8859-15"
		case "10":
			preparsed = "iso-8859-16"
		}
	}

	// missing substitutions
	switch charset {
	case "csutf8", "iso-utf-8", "utf8mb4":
		preparsed = "utf-8"

	case "eucjp", "ibm-eucjp":
		preparsed = "euc-jp"
	case "euckr", "ibm-euckr", "cp949":
		preparsed = "euc-kr"
	case "euccn", "ibm-euccn":
		preparsed = "gbk"
	case "zht16mswin950", "cp950":
		preparsed = "big5"

	case "csascii",
		"ansi_x3.4-1968",
		"ansi_x3.4-1986",
		"ansi_x3.110-1983",
		"cp850",
		"cp858",
		"us",
		"iso646",
		"iso-646",
		"iso646-us",
		"iso_646.irv:1991",
		"cp367",
		"ibm367",
		"ibm-367",
		"iso-ir-6":
		preparsed = "windows-1252"

	case "ibm852":
		preparsed = "iso-8859-2"
	case "iso-ir-199", "iso-celtic":
		preparsed = "iso-8859-14"
	case "iso-ir-226":
		preparsed = "iso-8859-16"

	case "macroman":
		preparsed = "macintosh"
	}

	enc, _ = htmlindex.Get(preparsed)
	if enc == nil {
		err = fmt.Errorf("Can not get encodig for '%s' (or '%s')", charset, preparsed)
	}
	return
}

func selectDecoder(charset string) (decoder *encoding.Decoder, err error) {
	var enc encoding.Encoding
	lcharset := strings.Trim(strings.ToLower(charset), " \t\r\n")
	switch lcharset {
	case "utf7", "utf-7", "unicode-1-1-utf-7":
		return NewUtf7Decoder(), nil
	default:
		enc, err = getEncoding(lcharset)
	}
	if err == nil {
		decoder = enc.NewDecoder()
	}
	return
}

func DecodeHeader(raw string) (decoded string, err error) {
	if decoded, err = wordDec.DecodeHeader(raw); err != nil {
		decoded = raw
	}
	return
}

func EncodeHeader(s string) string {
	return mime.QEncoding.Encode("utf-8", s)
}

func DecodeCharset(original []byte, parameters map[string]string) ([]byte, error) {
	charset, ok := parameters["charset"]
	decoder, err := selectDecoder(charset)
	if len(original) == 0 || !ok || decoder == nil {
		return original, err
	}

	utf8 := make([]byte, len(original))
	nDst, nSrc, err := decoder.Transform(utf8, original, false)
	for err == transform.ErrShortDst {
		utf8 = make([]byte, (nDst/nSrc+1)*len(original))
		nDst, nSrc, err = decoder.Transform(utf8, original, false)
	}
	if err != nil {
		return original, err
	}
	utf8 = bytes.Trim(utf8, "\x00")

	return utf8, nil
}

func DecodeContentEncoding(r io.Reader, contentEncoding string) (d io.Reader) {
	switch strings.ToLower(contentEncoding) {
	case "quoted-printable":
		d = quotedprintable.NewReader(r)
	case "base64":
		d = base64.NewDecoder(base64.StdEncoding, r)
	case "7bit", "8bit", "binary", "": // Nothing to do
		d = r
	}
	return
}
