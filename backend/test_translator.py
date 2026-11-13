import unittest

from backend.translator import (
    Translator,
    _glossary_hits,
    _is_math_line,
    build_lines_prompt,
    build_translation_prompt,
    normalize_target_language,
    parse_numbered_translations,
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

    def test_auto_language_is_treated_as_automatic(self):
        self.assertIsNone(normalize_target_language("auto"))
        self.assertIsNone(normalize_target_language("AUTO"))

    def test_prompt_includes_matching_glossary_terms(self):
        prompt = build_translation_prompt(
            "zh",
            "The GPU is fast.",
            glossary=[
                {"term": "GPU", "translation": "图形处理器", "source": "显卡手册"},
                {"term": "CPU", "translation": "中央处理器", "source": ""},
            ],
        )
        self.assertIn("GPU => 图形处理器", prompt)
        self.assertNotIn("CPU => 中央处理器", prompt)

    def test_glossary_hits_only_for_present_terms(self):
        hits = _glossary_hits(
            "GPU and LLM",
            [
                {"term": "GPU", "translation": "图形处理器", "source": "手册"},
                {"term": "CPU", "translation": "中央处理器", "source": ""},
            ],
        )
        self.assertEqual(len(hits), 1)
        self.assertEqual(hits[0]["term"], "GPU")
        self.assertEqual(hits[0]["source"], "手册")

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

    def test_long_text_is_chunked_and_joined(self):
        long_text = "这是一段用于验证长文分块翻译的中文文本。" * 200

        result, error = self.translator.translate(long_text, "vi")

        self.assertIsNone(error)
        self.assertGreater(len(self.translator._model.prompts), 1)
        expected = "\n".join(
            f"result-{i + 1}" for i in range(len(self.translator._model.prompts))
        )
        self.assertEqual(result, expected)

    def test_short_text_stays_single_inference(self):
        result, error = self.translator.translate("你好世界", "vi")

        self.assertIsNone(error)
        self.assertEqual(len(self.translator._model.prompts), 1)
        self.assertEqual(result, "result-1")

    def test_device_info_structure(self):
        info = self.translator.device_info
        self.assertIn("configured", info)
        self.assertIn("actual", info)
        self.assertIn("gpu_layers", info)


class LinePromptTests(unittest.TestCase):
    def test_plain_prompt_drops_numbering_and_states_line_count(self):
        prompt = build_lines_prompt(
            "zh", ["First line", "Second line"], numbered=False
        )

        self.assertIn("exactly 2 lines", prompt)
        self.assertNotIn("[1]", prompt)
        self.assertTrue(prompt.endswith("\n\nFirst line\nSecond line"))

    def test_numbered_prompt_keeps_markers(self):
        prompt = build_lines_prompt("zh", ["First line"])

        self.assertTrue(prompt.endswith("\n\n[1] First line"))

    def test_math_line_detection(self):
        self.assertTrue(_is_math_line("The total energy is E = mc2."))
        self.assertTrue(_is_math_line("函数 f(x) = x2 + 2x + 1 在 x = -1 处取最小值"))
        self.assertTrue(_is_math_line(r"\frac{a}{b} + \sqrt{x}"))
        self.assertFalse(_is_math_line("The quick brown fox jumps over the lazy dog."))

    def test_math_prompt_adds_latex_instruction(self):
        math_prompt = build_lines_prompt("zh", ["E = mc2"], math=True)
        plain_prompt = build_lines_prompt("zh", ["Hello"])

        self.assertIn("LaTeX inline math", math_prompt)
        self.assertNotIn("LaTeX inline math", plain_prompt)

    def test_parser_accepts_plain_one_line_per_input_output(self):
        parsed = parse_numbered_translations("第一行\n第二行\n", 2)

        self.assertEqual(parsed, ["第一行", "第二行"])

    def test_parser_rejects_plain_output_with_wrong_line_count(self):
        self.assertIsNone(parse_numbered_translations("第一行\n第二行", 3))

    def test_translate_lines_uses_plain_prompt_first(self):
        class PlainModel:
            def __init__(self):
                self.prompts = []

            def create_chat_completion(self, messages, **kwargs):
                self.prompts.append(messages[0]["content"])
                return {"choices": [{"message": {"content": "译文一\n译文二"}}]}

        translator = Translator()
        translator.ready = True
        translator._model = PlainModel()

        results, error = translator.translate_lines(["one", "two"], "zh")

        self.assertIsNone(error)
        self.assertEqual(results, ["译文一", "译文二"])
        self.assertEqual(len(translator._model.prompts), 1)
        self.assertNotIn("[1]", translator._model.prompts[0])

    def test_translate_lines_falls_back_to_numbered_prompt(self):
        class NumberedFallbackModel:
            def __init__(self):
                self.prompts = []

            def create_chat_completion(self, messages, **kwargs):
                prompt = messages[0]["content"]
                self.prompts.append(prompt)
                if "[1]" not in prompt:
                    return {"choices": [{"message": {"content": "只有一行"}}]}
                return {
                    "choices": [
                        {"message": {"content": "[1] 译文一\n[2] 译文二"}}
                    ]
                }

        translator = Translator()
        translator.ready = True
        translator._model = NumberedFallbackModel()

        results, error = translator.translate_lines(["one", "two"], "zh")

        self.assertIsNone(error)
        self.assertEqual(results, ["译文一", "译文二"])
        self.assertEqual(len(translator._model.prompts), 2)
        self.assertNotIn("[1]", translator._model.prompts[0])
        self.assertIn("[1]", translator._model.prompts[1])

    def test_translate_lines_can_bypass_cache(self):
        class CountingModel:
            def __init__(self):
                self.calls = 0

            def create_chat_completion(self, messages, **kwargs):
                self.calls += 1
                return {"choices": [{"message": {"content": f"译文{self.calls}"}}]}

        translator = Translator()
        translator.ready = True
        translator._model = CountingModel()

        first, _ = translator.translate_lines(["hello"], "zh")
        cached, _ = translator.translate_lines(["hello"], "zh")
        refreshed, _ = translator.translate_lines(["hello"], "zh", use_cache=False)

        self.assertEqual(first, ["译文1"])
        self.assertEqual(cached, ["译文1"])
        self.assertEqual(refreshed, ["译文2"])
        self.assertEqual(translator._model.calls, 2)


if __name__ == "__main__":
    unittest.main()

