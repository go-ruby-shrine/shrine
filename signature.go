// Copyright (c) the go-ruby-shrine/shrine authors
//
// SPDX-License-Identifier: BSD-3-Clause

package shrine

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/crc32"
	"strconv"
)

// SignatureAlgorithm selects the digest function for [Signature], mirroring the
// gem's signature plugin `algorithm` argument.
type SignatureAlgorithm string

// The algorithms the gem's signature plugin supports.
const (
	MD5    SignatureAlgorithm = "md5"
	SHA1   SignatureAlgorithm = "sha1"
	SHA256 SignatureAlgorithm = "sha256"
	SHA384 SignatureAlgorithm = "sha384"
	SHA512 SignatureAlgorithm = "sha512"
	CRC32  SignatureAlgorithm = "crc32"
)

// SignatureFormat selects the encoding of the raw digest, mirroring the gem's
// `format:` option.
type SignatureFormat string

// The output encodings the gem's signature plugin supports.
const (
	// Hex hex-encodes the digest (the gem's default).
	Hex SignatureFormat = "hex"
	// Base64 base64-encodes the digest (standard padded alphabet).
	Base64 SignatureFormat = "base64"
	// None returns the raw digest bytes as a string.
	None SignatureFormat = "none"
)

// ErrUnknownAlgorithm is returned (wrapped) by [Signature] for an unsupported
// algorithm, mirroring the gem raising on an unknown algorithm.
var ErrUnknownAlgorithm = fmt.Errorf("shrine: unknown signature algorithm")

// ErrUnknownFormat is returned (wrapped) by [Signature] for an unsupported
// format.
var ErrUnknownFormat = fmt.Errorf("shrine: unknown signature format")

// Signature computes the digest of data under algo and encodes it per format,
// mirroring `Shrine.signature(io, algorithm, format:)`.
//
// The crc32 case reproduces the gem faithfully: its raw digest is the checksum
// rendered as a *decimal string* (e.g. "222957957"), so [None] returns that
// string and [Hex]/[Base64] encode the bytes of that decimal string — not the
// four checksum bytes. The cryptographic algorithms digest to raw bytes as
// usual.
func Signature(data []byte, algo SignatureAlgorithm, format SignatureFormat) (string, error) {
	raw, err := rawDigest(data, algo)
	if err != nil {
		return "", err
	}
	switch format {
	case Hex:
		return hex.EncodeToString(raw), nil
	case Base64:
		return base64.StdEncoding.EncodeToString(raw), nil
	case None:
		return string(raw), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownFormat, format)
	}
}

// rawDigest returns the raw digest bytes for algo (see [Signature] for the
// crc32 quirk).
func rawDigest(data []byte, algo SignatureAlgorithm) ([]byte, error) {
	if algo == CRC32 {
		sum := crc32.ChecksumIEEE(data)
		return []byte(strconv.FormatUint(uint64(sum), 10)), nil
	}
	var h hash.Hash
	switch algo {
	case MD5:
		h = md5.New()
	case SHA1:
		h = sha1.New()
	case SHA256:
		h = sha256.New()
	case SHA384:
		h = sha512.New384()
	case SHA512:
		h = sha512.New()
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownAlgorithm, algo)
	}
	h.Write(data)
	return h.Sum(nil), nil
}

// SignatureMetadata is the signature plugin composed with add_metadata: it
// registers an extractor that stores the digest of every upload under Key.
// It is the Go analogue of the common `add_metadata(:md5) { signature(io,
// :md5) }` recipe. Enable it with [Shrine.Plugin]:
//
//	s.Plugin(&shrine.SignatureMetadata{Key: "md5", Algorithm: shrine.MD5, Format: shrine.Hex})
type SignatureMetadata struct {
	// Key is the metadata key to store the signature under (e.g. "md5").
	Key string
	// Algorithm and Format select the digest and its encoding; a zero Format
	// means [Hex].
	Algorithm SignatureAlgorithm
	Format    SignatureFormat
}

// Configure registers the signing [MetadataExtractor]. An unsupported
// algorithm/format yields a null value under Key rather than failing the
// upload, so a misconfiguration is visible in the metadata without aborting.
func (p *SignatureMetadata) Configure(s *Shrine) {
	format := p.Format
	if format == "" {
		format = Hex
	}
	s.AddMetadataKey(p.Key, func(data []byte, _ Metadata) any {
		sig, err := Signature(data, p.Algorithm, format)
		if err != nil {
			return nil
		}
		return sig
	})
}
