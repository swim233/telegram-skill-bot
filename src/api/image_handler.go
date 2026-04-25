package api

import (
	"encoding/base64"
)

func ImageToBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
