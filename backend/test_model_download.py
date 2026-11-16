import json
import os
import tempfile
import unittest
from unittest.mock import patch

from backend.translator import (
    MODEL_FILENAME,
    MODEL_ID,
    MODELSCOPE_MODEL_ID,
    UnsafeModelURLError,
    _read_model_download_config,
    _request_with_validated_redirects,
    _resolve_model_url,
    _validate_model_url,
)


class _FakeResponse:
    def __init__(self, status_code, headers=None):
        self.status_code = status_code
        self.headers = headers or {}
        self.closed = False

    def close(self):
        self.closed = True


class _FakeSession:
    def __init__(self, responses):
        self._responses = list(responses)
        self.calls = []

    def request(self, method, url, allow_redirects=True, **kwargs):
        self.calls.append((method, url, allow_redirects, kwargs))
        return self._responses.pop(0)


class DownloadURLValidationTests(unittest.TestCase):
    def test_rejects_non_http_schemes(self):
        for url in (
            "file:///C:/model.gguf",
            "ftp://example.com/model.gguf",
            "javascript:alert(1)",
        ):
            with self.subTest(url=url):
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

    def test_rejects_redirect_to_private_target(self):
        session = _FakeSession([
            _FakeResponse(302, {"Location": "http://127.0.0.1/secret.gguf"}),
        ])
        with self.assertRaises(UnsafeModelURLError):
            _request_with_validated_redirects(
                session, "GET", "https://1.1.1.1/model.gguf"
            )
        self.assertEqual(len(session.calls), 1)
        self.assertFalse(session.calls[0][2])

    def test_follows_safe_redirect_with_tls_verification_enabled(self):
        final_resp = _FakeResponse(200)
        session = _FakeSession([
            _FakeResponse(302, {"Location": "https://8.8.8.8/real.gguf"}),
            final_resp,
        ])
        got = _request_with_validated_redirects(
            session, "HEAD", "https://1.1.1.1/model.gguf", timeout=(15, 30)
        )
        self.assertIs(got, final_resp)
        self.assertEqual(len(session.calls), 2)
        self.assertNotIn("verify", session.calls[0][3])

    def test_rejects_dns_resolving_to_loopback(self):
        fake_addr = [(2, 1, 6, "", ("127.0.0.1", 443))]
        with patch("backend.translator.socket.getaddrinfo", return_value=fake_addr):
            with self.assertRaises(UnsafeModelURLError):
                _validate_model_url("https://rebind.example.com/model.gguf")


class ModelDownloadURLTests(unittest.TestCase):
    def test_defaults_to_modelscope_mirror(self):
        url, label = _resolve_model_url("", "")
        self.assertEqual(
            url,
            f"https://modelscope.cn/models/{MODELSCOPE_MODEL_ID}/resolve/master/{MODEL_FILENAME}",
        )
        self.assertEqual(label, "魔搭 ModelScope")

    def test_huggingface_source_uses_official_endpoint(self):
        url, label = _resolve_model_url("huggingface")
        self.assertEqual(
            url,
            f"https://huggingface.co/{MODEL_ID}/resolve/main/{MODEL_FILENAME}",
        )
        self.assertEqual(label, "HuggingFace 官网")

    def test_custom_mirror_root_resolves_model_path(self):
        url, label = _resolve_model_url("custom", "https://hf-mirror.example.com/")
        self.assertEqual(
            url,
            f"https://hf-mirror.example.com/{MODEL_ID}/resolve/main/{MODEL_FILENAME}",
        )
        self.assertEqual(label, "自定义镜像源")

    def test_custom_direct_gguf_url_is_preserved(self):
        direct = "https://cdn.example.com/models/custom-Hy-MT2.gguf"
        url, label = _resolve_model_url("custom", direct)
        self.assertEqual(url, direct)
        self.assertEqual(label, "自定义下载链接")

    def test_read_model_download_config_reads_settings_and_falls_back(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            settings_file = os.path.join(tmpdir, "settings.json")
            with patch("backend.translator.settings_path", return_value=settings_file):
                self.assertEqual(_read_model_download_config(), ("modelscope", ""))

                with open(settings_file, "w", encoding="utf-8") as f:
                    json.dump(
                        {
                            "model_download_source": "custom",
                            "model_download_base_url": "https://mirror.example.com/",
                        },
                        f,
                    )
                self.assertEqual(
                    _read_model_download_config(),
                    ("custom", "https://mirror.example.com"),
                )

                with open(settings_file, "w", encoding="utf-8") as f:
                    json.dump(
                        {
                            "model_download_source": "custom",
                            "model_download_base_url": "javascript:alert(1)",
                        },
                        f,
                    )
                self.assertEqual(_read_model_download_config(), ("modelscope", ""))


if __name__ == "__main__":
    unittest.main()

