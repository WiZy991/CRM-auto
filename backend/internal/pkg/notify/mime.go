package notify

import "mime"

// mimeEncode кодирует строку для заголовка письма по RFC 2047.
func mimeEncode(value string) string {
	return mime.BEncoding.Encode("UTF-8", value)
}
