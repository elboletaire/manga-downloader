// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var (
	wpMangaProtectorDataRe  = regexp.MustCompile(`chapter_data\s*=\s*'(.*?)'\s*;`)
	wpMangaProtectorNonceRe = regexp.MustCompile(`wpmangaprotectornonce\s*=\s*'(\w+)'`)
	// wpMangaProtectorNoticeRe matches the notice pages sites append to every
	// chapter (hpm-notice.jpg, zannouncement-urgent.jpg), not real pages.
	wpMangaProtectorNoticeRe = regexp.MustCompile(`(?i)notice|announcement`)
)

// wpMangaProtectorImages returns the page URLs of a chapter whose images the
// "wp-manga-chapter-images-protection" Madara plugin hides in an encrypted
// blob. ok is false when the page doesn't carry one, which costs a lookup.
func wpMangaProtectorImages(doc *goquery.Document) (urls []string, ok bool, err error) {
	script := doc.Find("script#chapter-protector-data")
	if script.Length() == 0 {
		return nil, false, nil
	}
	urls, err = decodeWPMangaProtector(script.Text())
	return urls, true, err
}

// decodeWPMangaProtector decrypts the plugin's inline script: a CryptoJS
// passphrase-mode blob whose passphrase is the page's nonce.
func decodeWPMangaProtector(script string) ([]string, error) {
	data := wpMangaProtectorDataRe.FindStringSubmatch(script)
	nonce := wpMangaProtectorNonceRe.FindStringSubmatch(script)
	if data == nil || nonce == nil {
		return nil, errors.New("chapter protector data not found")
	}

	var blob struct {
		CT string `json:"ct"`
		S  string `json:"s"`
	}
	if err := json.Unmarshal([]byte(data[1]), &blob); err != nil {
		return nil, err
	}
	ct, err := base64.StdEncoding.DecodeString(blob.CT)
	if err != nil {
		return nil, err
	}
	salt, err := hex.DecodeString(blob.S)
	if err != nil {
		return nil, err
	}

	plain, err := decryptSalted(ct, []byte(nonce[1]), salt)
	if err != nil {
		return nil, err
	}

	// the plaintext is a JSON string holding the JSON array
	var inner string
	if err := json.Unmarshal(plain, &inner); err == nil {
		plain = []byte(inner)
	}
	var all []string
	if err := json.Unmarshal(plain, &all); err != nil {
		return nil, err
	}

	urls := make([]string, 0, len(all))
	for _, u := range all {
		if wpMangaProtectorNoticeRe.MatchString(path.Base(u)) {
			continue
		}
		urls = append(urls, u)
	}
	return urls, nil
}

// decryptSalted decrypts AES-256-CBC data with a key and IV derived from the
// passphrase and salt the way OpenSSL's EVP_BytesToKey does (MD5, 1 round).
func decryptSalted(ct, pass, salt []byte) ([]byte, error) {
	var derived, prev []byte
	for len(derived) < 48 {
		h := md5.New()
		h.Write(prev)
		h.Write(pass)
		h.Write(salt)
		prev = h.Sum(nil)
		derived = append(derived, prev...)
	}

	block, err := aes.NewCipher(derived[:32])
	if err != nil {
		return nil, err
	}
	if len(ct) == 0 || len(ct)%aes.BlockSize != 0 {
		return nil, errors.New("invalid ciphertext length")
	}
	out := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, derived[32:48]).CryptBlocks(out, ct)

	pad := int(out[len(out)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(out) || strings.Trim(string(out[len(out)-pad:]), string(rune(pad))) != "" {
		return nil, errors.New("invalid padding, wrong passphrase?")
	}
	return out[:len(out)-pad], nil
}
