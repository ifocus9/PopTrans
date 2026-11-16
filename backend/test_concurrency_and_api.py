import threading
import time
import unittest

from fastapi.testclient import TestClient

from backend import api_server
from backend.ocr_service import OCRService
from backend.translator import Translator


class _TrackingModel:
    def __init__(self):
        self._guard = threading.Lock()
        self.overlap_detected = False
        self.calls = 0

    def create_chat_completion(self, messages, stream=False, **kwargs):
        entered = self._guard.acquire(blocking=False)
        if not entered:
            self.overlap_detected = True
            raise RuntimeError("concurrent create_chat_completion call")
        try:
            self.calls += 1
            call_id = self.calls
            if not stream:
                time.sleep(0.03)
                return {"choices": [{"message": {"content": f"ok-{call_id}"}}]}

            def _gen():
                yield {"choices": [{"delta": {"content": f"part1-{call_id}"}}]}
                time.sleep(0.02)
                yield {"choices": [{"delta": {"content": f"part2-{call_id}"}}]}

            return _gen()
        finally:
            self._guard.release()


class _TrackingStreamTranslatorModel:
    def __init__(self):
        self.in_flight = 0
        self.overlap_detected = False
        self.calls = 0

    def create_chat_completion(self, messages, stream=False, **kwargs):
        self.calls += 1
        call_id = self.calls

        def _gen():
            self.in_flight += 1
            if self.in_flight > 1:
                self.overlap_detected = True
            try:
                yield {"choices": [{"delta": {"content": f"c{call_id}a"}}]}
                time.sleep(0.03)
                yield {"choices": [{"delta": {"content": f"c{call_id}b"}}]}
            finally:
                self.in_flight -= 1

        return _gen()


class _TrackingOCREngine:
    def __init__(self):
        self._guard = threading.Lock()
        self.overlap_detected = False

    def __call__(self, img):
        if not self._guard.acquire(blocking=False):
            self.overlap_detected = True
            raise RuntimeError("concurrent OCR engine call")
        try:
            time.sleep(0.03)
            return [[[[0, 0], [10, 0], [10, 10], [0, 10]], "hello", 0.99]], None
        finally:
            self._guard.release()


class ConcurrencyLockTests(unittest.TestCase):
    def test_translator_serializes_concurrent_complete_calls(self):
        translator = Translator()
        translator.ready = True
        model = _TrackingModel()
        translator._model = model

        errors = []

        def worker(idx):
            _, err = translator.translate(f"hello world {idx}", "zh")
            if err:
                errors.append(err)

        threads = [threading.Thread(target=worker, args=(i,)) for i in range(4)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()

        self.assertFalse(model.overlap_detected)
        self.assertEqual(errors, [])

    def test_chat_completion_stream_yields_all_chunks_and_serializes(self):
        translator = Translator()
        translator.ready = True
        model = _TrackingStreamTranslatorModel()
        translator._model = model

        long_text = "This is a long sentence for chunked streaming translation test. " * 200
        output = "".join(translator.chat_completion_stream(long_text, "zh"))
        self.assertGreater(model.calls, 1)
        for i in range(1, model.calls + 1):
            self.assertIn(f"c{i}a", output)
            self.assertIn(f"c{i}b", output)

        results = []

        def stream_worker(idx):
            results.append("".join(translator.chat_completion_stream(f"short {idx}", "zh")))

        threads = [threading.Thread(target=stream_worker, args=(i,)) for i in range(4)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()

        self.assertFalse(model.overlap_detected)
        self.assertEqual(len(results), 4)

    def test_ocr_service_serializes_concurrent_engine_calls(self):
        from PIL import Image

        service = OCRService()
        engine = _TrackingOCREngine()
        service._engine = engine
        img = Image.new("RGB", (64, 64), color=(255, 255, 255))

        results = []

        def worker():
            results.append(service.recognize_image(img))

        threads = [threading.Thread(target=worker) for _ in range(4)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()

        self.assertFalse(engine.overlap_detected)
        self.assertEqual(len(results), 4)


class APIServerUploadTests(unittest.TestCase):
    def setUp(self):
        self.client = TestClient(api_server.app)

    def test_rejects_missing_or_non_image_content_type_cleanly(self):
        resp = self.client.post(
            "/api/v1/ocr",
            files={"file": ("note.txt", b"hello", "text/plain")},
        )
        self.assertEqual(resp.status_code, 400)

        resp2 = self.client.post(
            "/api/v1/ocr_translate",
            files={"file": ("raw.bin", b"hello", "")},
        )
        self.assertEqual(resp2.status_code, 400)

    def test_rejects_corrupted_empty_and_oversized_images(self):
        resp_empty = self.client.post(
            "/api/v1/ocr",
            files={"file": ("empty.png", b"", "image/png")},
        )
        self.assertEqual(resp_empty.status_code, 400)

        resp_bad = self.client.post(
            "/api/v1/ocr",
            files={"file": ("bad.png", b"not a real image", "image/png")},
        )
        self.assertEqual(resp_bad.status_code, 400)

        oversized = b"0" * (api_server.MAX_UPLOAD_IMAGE_BYTES + 1)
        resp_big = self.client.post(
            "/api/v1/ocr",
            files={"file": ("big.png", oversized, "image/png")},
        )
        self.assertEqual(resp_big.status_code, 413)


if __name__ == "__main__":
    unittest.main()

