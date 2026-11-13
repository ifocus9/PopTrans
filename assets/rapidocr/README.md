# Korean and Japanese OCR models

These files enable Korean and Japanese text recognition. They are copied into
`models/rapidocr/` at build time and shipped next to the other RapidOCR models.

| File | Source | License |
| --- | --- | --- |
| `korean_PP-OCRv3_rec_infer.onnx` | PaddleOCR Korean PP-OCRv3 recognition model, ONNX conversion from the MindSpore/PaddleOCR model mirror (`download-mindspore.osinfra.cn/toolkits/mindocr/ppocr/rec/svtr/`) | Apache-2.0 |
| `korean_dict.txt` | PaddleOCR `ppocr/utils/dict/korean_dict.txt` | Apache-2.0 |
| `japanese_PP-OCRv3_rec_infer.onnx` | PaddleOCR Japanese PP-OCRv3 recognition model, ONNX conversion from the same MindSpore/PaddleOCR mirror | Apache-2.0 |
| `japanese_dict.txt` | PaddleOCR `ppocr/utils/dict/japan_dict.txt` | Apache-2.0 |

The detection model stays the shared `ch_PP-OCRv4_det_infer.onnx`; only the
recognition model and dictionary differ. `OCRService` selects these models when
the source language is Korean/Japanese, or automatically when the default model
finds no meaningful text or the result contains Kana/Hangul.

Latin-script languages (English, French, German, Spanish, etc.) are handled by
the default Chinese/English recognizer, whose dictionary includes Latin letters
and punctuation. Translation itself supports the full language list in the
settings panel.
