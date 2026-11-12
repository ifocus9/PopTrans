import unittest

from backend.translator import (
    MODEL_FILENAME,
    MODEL_SOURCE_CUSTOM,
    MODEL_SOURCE_HUGGINGFACE,
    MODEL_SOURCE_MODELSCOPE,
    UnsafeModelURLError,
    _resolve_model_url,
    _validate_model_url,
    build_lines_prompt,
    parse_numbered_translations,
)


class ModelDownloadURLTests(unittest.TestCase):
    def test_modelscope_is_the_default_source(self):
        url = _resolve_model_url(MODEL_SOURCE_MODELSCOPE)
        self.assertTrue(url.startswith("https://modelscope.cn/models/Tencent-Hunyuan/"))
        self.assertTrue(url.endswith(f"/resolve/master/{MODEL_FILENAME}"))

    def test_huggingface_source(self):
        url = _resolve_model_url(MODEL_SOURCE_HUGGINGFACE)
        self.assertEqual(
            url,
            f"https://huggingface.co/tencent/Hy-MT2-1.8B-GGUF/resolve/main/{MODEL_FILENAME}",
        )

    def test_custom_mirror_root_builds_hf_style_url(self):
        url = _resolve_model_url(MODEL_SOURCE_CUSTOM, "https://hf-mirror.com/")
        self.assertEqual(
            url,
            f"https://hf-mirror.com/tencent/Hy-MT2-1.8B-GGUF/resolve/main/{MODEL_FILENAME}",
        )

    def test_custom_full_file_url_is_kept(self):
        full = "https://mirror.example.com/files/Hy-MT2-1.8B-Q4_K_M.gguf"
        self.assertEqual(_resolve_model_url(MODEL_SOURCE_CUSTOM, full), full)


class DownloadURLValidationTests(unittest.TestCase):
    def test_rejects_non_http_schemes(self):
        for url in (
            "file:///C:/model.gguf",
            "ftp://example.com/model.gguf",
            "javascript:alert(1)",
        ):
            with self.assertRaises(UnsafeModelURLError):
                _validate_model_url(url)

    def test_rejects_localhost_loopback_private_and_reserved(self):
        blocked = (
            "http://localhost/model.gguf",
            "http://app.localhost/model.gguf",
            "http://127.0.0.1/model.gguf",
            "http://10.1.2.3/model.gguf",
            "http://172.16.0.9/model.gguf",
            "http://192.168.1.5/model.gguf",
            "http://169.254.169.254/model.gguf",
            "http://[::1]/model.gguf",
            "http://[fe80::1]/model.gguf",
        )
        for url in blocked:
            with self.subTest(url=url):
                with self.assertRaises(UnsafeModelURLError):
                    _validate_model_url(url)

    def test_accepts_public_ip_and_https(self):
        self.assertEqual(
            _validate_model_url("https://1.1.1.1/model.gguf"),
            "https://1.1.1.1/model.gguf",
        )
        self.assertEqual(
            _validate_model_url("http://8.8.8.8/model.gguf"),
            "http://8.8.8.8/model.gguf",
        )


class LineTranslationPromptTests(unittest.TestCase):
    def test_prompt_numbers_lines(self):
        prompt = build_lines_prompt("zh", ["hello", "world"])
        self.assertIn("[1] hello", prompt)
        self.assertIn("[2] world", prompt)
        self.assertIn("Chinese", prompt)

    def test_parses_numbered_output(self):
        parsed = parse_numbered_translations("[1] 你好\n[2] 世界", 2)
        self.assertEqual(parsed, ["你好", "世界"])

    def test_parses_unmarked_output_when_line_count_matches(self):
        parsed = parse_numbered_translations("你好\n世界", 2)
        self.assertEqual(parsed, ["你好", "世界"])

    def test_keeps_none_for_missing_lines(self):
        parsed = parse_numbered_translations("[2] 世界", 2)
        self.assertEqual(parsed, [None, "世界"])

    def test_returns_none_when_unparseable(self):
        self.assertIsNone(parse_numbered_translations("只有一行", 2))


if __name__ == "__main__":
    unittest.main()
