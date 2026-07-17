import unittest
from pathlib import Path

from backend.runtime_paths import application_dir, model_path, settings_path


class RuntimePathsTest(unittest.TestCase):
    def test_source_mode_uses_project_root(self):
        project_root = Path(__file__).resolve().parent.parent

        self.assertEqual(application_dir(), project_root)
        self.assertEqual(settings_path(), project_root / "settings.json")
        self.assertEqual(
            Path(model_path("Hy-MT2-1.8B-GGUF", "model.gguf")),
            project_root / "models" / "Hy-MT2-1.8B-GGUF" / "model.gguf",
        )


if __name__ == "__main__":
    unittest.main()
