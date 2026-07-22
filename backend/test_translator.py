import unittest

from backend.translator import (
    Translator,
    build_translation_prompt,
    normalize_target_language,
)


class FakeModel:
    def __init__(self):
        self.prompts = []

    def create_chat_completion(self, messages, **kwargs):
        prompt = messages[0]["content"]
        self.prompts.append(prompt)
        return {
            "choices": [
                {
                    "message": {
                        "content": f"result-{len(self.prompts)}",
                    }
                }
            ]
        }


class TranslatorLanguageTests(unittest.TestCase):
    def setUp(self):
        self.translator = Translator()
        self.translator.ready = True
        self.translator._model = FakeModel()

    def test_normalizes_language_codes_names_and_locales(self):
        self.assertEqual(normalize_target_language("vi"), "vi")
        self.assertEqual(normalize_target_language("vi-VN"), "vi")
        self.assertEqual(normalize_target_language("越南语"), "vi")
        self.assertEqual(normalize_target_language("Vietnamese"), "vi")
        self.assertEqual(normalize_target_language("zh-Hant"), "zh-hant")
        self.assertIsNone(normalize_target_language(None))

    def test_rejects_unsupported_language(self):
        with self.assertRaisesRegex(ValueError, "不支持的目标语言"):
            normalize_target_language("unknown")

    def test_translates_to_explicit_vietnamese(self):
        result, error = self.translator.translate("你好", "vi")

        self.assertIsNone(error)
        self.assertEqual(result, "result-1")
        self.assertIn("into Vietnamese", self.translator._model.prompts[0])
        self.assertTrue(self.translator._model.prompts[0].endswith("\n\n你好"))

    def test_builds_official_prompt_with_full_language_name(self):
        prompt = build_translation_prompt("vi", "欢迎使用")

        self.assertIn("into Vietnamese", prompt)
        self.assertNotIn("vi", prompt)
        self.assertTrue(prompt.endswith("\n\n欢迎使用"))

    def test_cache_is_isolated_by_target_language(self):
        first, _ = self.translator.translate("你好", "vi")
        second, _ = self.translator.translate("你好", "en")
        cached, _ = self.translator.translate("你好", "vi")

        self.assertEqual(first, "result-1")
        self.assertEqual(second, "result-2")
        self.assertEqual(cached, first)
        self.assertEqual(len(self.translator._model.prompts), 2)

    def test_automatic_mode_keeps_existing_direction(self):
        _, error = self.translator.translate("你好")

        self.assertIsNone(error)
        self.assertIn("into English", self.translator._model.prompts[0])


if __name__ == "__main__":
    unittest.main()
