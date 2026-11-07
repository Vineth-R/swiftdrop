# Receipt fonts

These unmodified fonts are bundled solely for server-generated PDF receipts.
They are licensed under the [SIL Open Font License 1.1](OFL.txt). The original
copyright and license metadata is also retained inside each font file.
The OFL text is embedded alongside the font assets in the service binary.

Source: the official [notofonts/noto-fonts](https://github.com/notofonts/noto-fonts)
repository, commit `ffebf8c1ee449e544955a7e813c54f9b73848eac`.
Each file comes from `hinted/ttf/<family>/<family>-Regular.ttf` at that commit;
the accompanying `OFL.txt` is the repository's root `LICENSE`.

| File                        | SHA-256                                                            |
| --------------------------- | ------------------------------------------------------------------ |
| NotoSans-Regular.ttf        | `b85c38ecea8a7cfb39c24e395a4007474fa5a4fc864f6ee33309eb4948d232d5` |
| NotoSansSinhala-Regular.ttf | `a966549310b2d3046ba25bbeef11985428ff9356fa656fd6118bc90fb82df031` |
| NotoSansTamil-Regular.ttf   | `6532db33b8b264abe3a098a40619feb489b5ddf5ab1d2b46e72b51eeb548001b` |
| NotoSansArabic-Regular.ttf  | `ceea25b464a656dc3b26849bab9356740401af62aedf1bfa8b7f0d9b75925b1b` |
| NotoSansHebrew-Regular.ttf  | `a7fa16fffb27bedb060a0866267c29e9859aeb9c21cc33f5b3aaf6eb062eca85` |

Receipts shape non-ASCII text with the pinned pure-Go `go-text/typesetting`
library, resolve script and bidirectional runs, wrap at shaping-cluster
boundaries, and draw actual glyph outlines. An embedded font and a `ToUnicode`
map provide a separate searchable logical-text layer, preserving original text
independently of ligatures and RTL display order. The output does not depend on
fonts installed on the host or on browser/network font downloads.

The tested scripts are Latin with accents, Sinhala, Tamil, Arabic and Hebrew.
This font set does not cover every Unicode script, CJK or color emoji. Unknown
glyphs remain represented by the font's missing-glyph outline while the exact
original code points remain in the searchable layer; universal font coverage is
not claimed. The application does not reject parcel names based on font coverage.

To generate inspection artifacts without committing generated PDFs:

```powershell
$env:M5_RECEIPT_ARTIFACT_DIR = Join-Path $PWD 'tmp/m5/receipt-artifacts'
go test ./apps/services/parcel/internal/parcels -run '^TestReceiptUnicode' -count=1
```

The optional independent inspector uses two PDF parsers, native font rendering
and a comparison image. These are disposable verification tools, not application
or CI dependencies. The versions used for the recorded inspection were pypdf
6.21.4, PyMuPDF 2.1 and Pillow 13.0.0:

```powershell
python -m pip install --target tmp/m5/receipt-python pypdf==6.21.4 pymupdf==2.1 pillow==13.0.0
$env:PYTHONPATH = Join-Path $PWD 'tmp/m5/receipt-python'
python apps/services/parcel/internal/parcels/testdata/inspect_receipts.py
```

The inspector checks exact original names with both parsers, the stored fee and
strict PDF parsing, then produces `tmp/m5/receipt-artifacts/comparison.png` for
visual comparison against native shaping of the same bundled font files. It
covers precomposed and decomposed Latin accents, Sinhala, Tamil, Arabic and Hebrew,
including combining marks in RTL names. Generated artifacts remain untracked.
