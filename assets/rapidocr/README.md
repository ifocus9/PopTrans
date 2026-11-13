# Korean OCR model

These files enable Korean text recognition. They are copied into
`models/rapidocr/` at build time and shipped next to the other RapidOCR models.

| File | Source | License |
| --- | --- | --- |
| `korean_PP-OCRv3_rec_infer.onnx` | PaddleOCR Korean PP-OCRv3 recognition model, ONNX conversion from the MindSpore/PaddleOCR model mirror (`download-mindspore.osinfra.cn/toolkits/mindocr/ppocr/rec/svtr/`) | Apache-2.0 |
| `korean_dict.txt` | PaddleOCR `ppocr/utils/dict/korean_dict.txt` | Apache-2.0 |

The detection model stays the shared `ch_PP-OCRv4_det_infer.onnx`; only the
recognition model and dictionary differ. `OCRService` selects this model when
the source language is Korean, or automatically when the default model finds
no meaningful text.
