import unittest

from backend.ocr_service import (
    _contains_hangul,
    _contains_kana,
    _is_japanese_language,
    _is_korean_language,
    _meaningful_text_ratio,
)


class OCRLanguageDetectionTests(unittest.TestCase):
    def test_japanese_language_aliases(self):
        for value in ("ja", "ja-JP", "Japanese", "日本語", "日语", "日語"):
            self.assertTrue(_is_japanese_language(value), value)
        for value in ("", None, "zh", "ko", "en", "japanese-ish"):
            self.assertFalse(_is_japanese_language(value), value)

    def test_korean_language_aliases(self):
        for value in ("ko", "ko-KR", "Korean", "한국어", "韩语", "韓語"):
            self.assertTrue(_is_korean_language(value), value)
        self.assertFalse(_is_korean_language("ja"))

    def test_kana_and_hangul_detection(self):
        self.assertTrue(_contains_kana("これはテストです"))
        self.assertTrue(_contains_kana("カタカナ"))
        self.assertFalse(_contains_kana("漢字文章"))
        self.assertTrue(_contains_hangul("한국어 문장"))
        self.assertFalse(_contains_hangul("中文句子"))

    def test_meaningful_text_ratio_ignores_punctuation(self):
        self.assertGreater(_meaningful_text_ratio("Hello world"), 0.8)
        self.assertLess(_meaningful_text_ratio("，。！？"), 0.2)


if __name__ == "__main__":
    unittest.main()
