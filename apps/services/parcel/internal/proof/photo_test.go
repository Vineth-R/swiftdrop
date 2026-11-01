package proof_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/proof"
)

func TestValidatePhoto(t *testing.T) {
	jpegData := encodePhoto(t, "image/jpeg", 2, 2)
	pngData := encodePhoto(t, "image/png", 2, 2)
	corruptPNG := bytes.Clone(pngData)
	idat := bytes.Index(corruptPNG, []byte("IDAT"))
	if idat < 0 {
		t.Fatal("PNG fixture has no IDAT chunk")
	}
	corruptPNG[idat+4] ^= 0xff
	corruptPNGHeader := bytes.Clone(pngData)
	corruptPNGHeader[29] ^= 0xff // Break the IHDR checksum.
	corruptJPEG := bytes.Clone(jpegData)
	corruptJPEG[4], corruptJPEG[5] = 0, 1 // Invalid first segment length.

	tests := []struct {
		name     string
		data     []byte
		declared string
		wantMIME string
		wantErr  error
	}{
		{name: "JPEG", data: jpegData, declared: "image/jpeg", wantMIME: "image/jpeg"},
		{name: "PNG", data: pngData, declared: "image/png", wantMIME: "image/png"},
		{name: "normalized MIME", data: jpegData, declared: " IMAGE/JPEG ", wantMIME: "image/jpeg"},
		{name: "MIME parameters", data: pngData, declared: "image/png; name=proof.png", wantMIME: "image/png"},
		{name: "nil", declared: "image/jpeg", wantErr: proof.ErrEmptyPhoto},
		{name: "empty", data: []byte{}, declared: "image/png", wantErr: proof.ErrEmptyPhoto},
		{name: "missing MIME", data: jpegData, wantErr: proof.ErrUnsupportedPhotoType},
		{name: "blank MIME", data: pngData, declared: " ", wantErr: proof.ErrUnsupportedPhotoType},
		{name: "malformed MIME", data: pngData, declared: "image", wantErr: proof.ErrUnsupportedPhotoType},
		{
			name: "malformed MIME parameter", data: pngData,
			declared: "image/png; name", wantErr: proof.ErrUnsupportedPhotoType,
		},
		{name: "generic MIME", data: jpegData, declared: "application/octet-stream", wantErr: proof.ErrUnsupportedPhotoType},
		{name: "JPEG alias", data: jpegData, declared: "image/jpg", wantErr: proof.ErrUnsupportedPhotoType},
		{name: "unsupported declaration", data: pngData, declared: "image/gif", wantErr: proof.ErrUnsupportedPhotoType},
		{name: "JPEG declared PNG", data: jpegData, declared: "image/png", wantErr: proof.ErrPhotoMIMETypeMismatch},
		{name: "PNG declared JPEG", data: pngData, declared: "image/jpeg", wantErr: proof.ErrPhotoMIMETypeMismatch},
		{name: "text spoof", data: []byte("not a JPEG"), declared: "image/jpeg", wantErr: proof.ErrUnsupportedPhotoType},
		{name: "binary spoof", data: []byte{0, 1, 2, 3}, declared: "image/png", wantErr: proof.ErrUnsupportedPhotoType},
		{
			name: "GIF spoof", data: []byte("GIF89a\x01\x00\x01\x00"),
			declared: "image/png", wantErr: proof.ErrUnsupportedPhotoType,
		},
		{
			name: "WebP spoof", data: []byte("RIFF\x10\x00\x00\x00WEBPVP8 "),
			declared: "image/jpeg", wantErr: proof.ErrUnsupportedPhotoType,
		},
		{
			name: "SVG spoof", data: []byte("<svg xmlns='http://www.w3.org/2000/svg'/>"),
			declared: "image/png", wantErr: proof.ErrUnsupportedPhotoType,
		},
		{name: "JPEG signature only", data: []byte{0xff, 0xd8, 0xff}, declared: "image/jpeg", wantErr: proof.ErrInvalidPhoto},
		{
			name: "PNG signature only", data: []byte("\x89PNG\r\n\x1a\n"),
			declared: "image/png", wantErr: proof.ErrInvalidPhoto,
		},
		{
			name: "JPEG signature over PNG data", data: append([]byte{0xff, 0xd8, 0xff}, pngData...),
			declared: "image/jpeg", wantErr: proof.ErrInvalidPhoto,
		},
		{
			name: "PNG signature over JPEG data", data: append([]byte("\x89PNG\r\n\x1a\n"), jpegData...),
			declared: "image/png", wantErr: proof.ErrInvalidPhoto,
		},
		{name: "corrupt JPEG header", data: corruptJPEG, declared: "image/jpeg", wantErr: proof.ErrInvalidPhoto},
		{name: "corrupt PNG header", data: corruptPNGHeader, declared: "image/png", wantErr: proof.ErrInvalidPhoto},
		{name: "corrupt PNG pixels", data: corruptPNG, declared: "image/png", wantErr: proof.ErrInvalidPhoto},
		{
			name: "JPEG missing end marker", data: jpegData[:len(jpegData)-2],
			declared: "image/jpeg", wantErr: proof.ErrInvalidPhoto,
		},
		{
			name: "JPEG missing final byte", data: jpegData[:len(jpegData)-1],
			declared: "image/jpeg", wantErr: proof.ErrInvalidPhoto,
		},
		{
			name: "PNG missing end chunk", data: pngData[:len(pngData)-12],
			declared: "image/png", wantErr: proof.ErrInvalidPhoto,
		},
		{
			name: "PNG missing final byte", data: pngData[:len(pngData)-1],
			declared: "image/png", wantErr: proof.ErrInvalidPhoto,
		},
		{
			name: "JPEG truncated body", data: jpegData[:len(jpegData)/2],
			declared: "image/jpeg", wantErr: proof.ErrInvalidPhoto,
		},
		{name: "PNG truncated body", data: pngData[:len(pngData)/2], declared: "image/png", wantErr: proof.ErrInvalidPhoto},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := bytes.Clone(tt.data)
			got, err := proof.ValidatePhoto(tt.data, tt.declared)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidatePhoto() error = %v, want %v", err, tt.wantErr)
			}
			if !bytes.Equal(tt.data, original) {
				t.Fatal("ValidatePhoto() modified the input")
			}
			if tt.wantErr != nil {
				if got != (proof.PhotoMetadata{}) {
					t.Fatalf("failed validation returned metadata: %+v", got)
				}
				return
			}
			want := proof.PhotoMetadata{MIMEType: tt.wantMIME, SizeBytes: len(tt.data)}
			if got != want {
				t.Fatalf("ValidatePhoto() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestValidatePhotoByteLimit(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr error
	}{
		{name: "one byte below limit", size: 5_242_879},
		{name: "exact limit", size: 5_242_880},
		{name: "one byte above limit", size: 5_242_881, wantErr: proof.ErrPhotoTooLarge},
	}
	for _, mimeType := range []string{"image/jpeg", "image/png"} {
		t.Run(mimeType, func(t *testing.T) {
			base := encodePhoto(t, mimeType, 1, 1)
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					data := padPhoto(t, base, mimeType, tt.size)
					// Every boundary fixture is a complete image with legal metadata,
					// including the over-limit fixture; size is its only violation.
					if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
						t.Fatalf("invalid boundary fixture: %v", err)
					}
					got, err := proof.ValidatePhoto(data, mimeType)
					if !errors.Is(err, tt.wantErr) {
						t.Fatalf("ValidatePhoto(%d bytes) error = %v, want %v", tt.size, err, tt.wantErr)
					}
					if tt.wantErr != nil {
						if got != (proof.PhotoMetadata{}) {
							t.Fatalf("oversize image returned metadata: %+v", got)
						}
						return
					}
					if got.SizeBytes != tt.size || got.MIMEType != mimeType {
						t.Fatalf("ValidatePhoto() metadata = %+v", got)
					}
				})
			}
		})
	}
}

func TestValidatePhotoDimensions(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		headerOnly bool
		wantErr    error
	}{
		{name: "width at limit", width: 8192, height: 1},
		{name: "height at limit", width: 1, height: 8192},
		{name: "pixels below limit", width: 3999, height: 4001},
		{name: "pixels at limit", width: 4000, height: 4000},
		{
			name: "width above limit before decode", width: 8193, height: 1,
			headerOnly: true, wantErr: proof.ErrPhotoDimensions,
		},
		{
			name: "height above limit before decode", width: 1, height: 8193,
			headerOnly: true, wantErr: proof.ErrPhotoDimensions,
		},
		{
			name: "pixels above limit before decode", width: 4000, height: 4001,
			headerOnly: true, wantErr: proof.ErrPhotoDimensions,
		},
		{name: "both safeguards exceeded", width: 65535, height: 65535, headerOnly: true, wantErr: proof.ErrPhotoDimensions},
		{name: "zero width", width: 0, height: 1, headerOnly: true, wantErr: proof.ErrInvalidPhoto},
		{name: "zero height", width: 1, height: 0, headerOnly: true, wantErr: proof.ErrInvalidPhoto},
	}
	for _, mimeType := range []string{"image/jpeg", "image/png"} {
		t.Run(mimeType, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					var data []byte
					if tt.headerOnly {
						data = photoHeader(t, mimeType, tt.width, tt.height)
						if errors.Is(tt.wantErr, proof.ErrPhotoDimensions) {
							cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
							if err != nil || cfg.Width != tt.width || cfg.Height != tt.height {
								t.Fatalf("dimension fixture header = %+v, error = %v", cfg, err)
							}
							// The body is deliberately absent. Getting a dimension error
							// rather than a content error verifies validation order without
							// allocating pixels for these untrusted, oversized headers.
						}
					} else {
						data = encodePhoto(t, mimeType, tt.width, tt.height)
					}
					got, err := proof.ValidatePhoto(data, mimeType)
					if !errors.Is(err, tt.wantErr) {
						t.Fatalf("ValidatePhoto(%dx%d) error = %v, want %v", tt.width, tt.height, err, tt.wantErr)
					}
					if tt.wantErr != nil {
						if got != (proof.PhotoMetadata{}) {
							t.Fatalf("invalid dimensions returned metadata: %+v", got)
						}
						return
					}
					if got.MIMEType != mimeType || got.SizeBytes != len(data) {
						t.Fatalf("ValidatePhoto() metadata = %+v", got)
					}
				})
			}
		})
	}
}

func encodePhoto(t *testing.T, mimeType string, width, height int) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	var err error
	if mimeType == "image/jpeg" {
		err = jpeg.Encode(&buf, img, nil)
	} else {
		err = png.Encode(&buf, img)
	}
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return buf.Bytes()
}

func padPhoto(t *testing.T, base []byte, mimeType string, size int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if mimeType == "image/png" {
		// Insert a private ancillary chunk immediately before IEND.
		payload := make([]byte, size-len(base)-12)
		buf.Write(base[:len(base)-12])
		buf.Write(pngChunk("paDd", payload))
		buf.Write(base[len(base)-12:])
	} else {
		// JPEG COM segment lengths include their two length bytes, but not
		// the marker. Split padding to stay within the 16-bit segment limit.
		buf.Write(base[:2])
		remaining := size - len(base)
		for remaining > 0 {
			segmentSize := min(remaining, 65537)
			if tail := remaining - segmentSize; tail > 0 && tail < 4 {
				segmentSize -= 4 - tail
			}
			buf.Write([]byte{0xff, 0xfe, byte((segmentSize - 2) >> 8), byte(segmentSize - 2)})
			buf.Write(make([]byte, segmentSize-4))
			remaining -= segmentSize
		}
		buf.Write(base[2:])
	}
	if buf.Len() != size {
		t.Fatalf("padded fixture has %d bytes, want %d", buf.Len(), size)
	}
	return buf.Bytes()
}

func photoHeader(t *testing.T, mimeType string, width, height int) []byte {
	t.Helper()
	data := encodePhoto(t, mimeType, 1, 1)
	if mimeType == "image/png" {
		ihdr := bytes.Clone(data[16:29])
		binary.BigEndian.PutUint32(ihdr[:4], uint32(width))
		binary.BigEndian.PutUint32(ihdr[4:8], uint32(height))
		return append(bytes.Clone(data[:8]), pngChunk("IHDR", ihdr)...)
	}
	sof := bytes.Index(data, []byte{0xff, 0xc0})
	if sof < 0 {
		t.Fatal("JPEG fixture has no baseline frame header")
	}
	binary.BigEndian.PutUint16(data[sof+5:sof+7], uint16(height))
	binary.BigEndian.PutUint16(data[sof+7:sof+9], uint16(width))
	// DecodeConfig reads the SOS marker but does not need its body.
	sos := bytes.Index(data, []byte{0xff, 0xda})
	if sos < 0 {
		t.Fatal("JPEG fixture has no scan header")
	}
	return data[:sos+4]
}

func pngChunk(kind string, payload []byte) []byte {
	chunk := make([]byte, len(payload)+12)
	binary.BigEndian.PutUint32(chunk[:4], uint32(len(payload)))
	copy(chunk[4:8], kind)
	copy(chunk[8:], payload)
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
	return chunk
}
