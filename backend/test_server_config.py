import unittest

from backend.server_config import DEFAULT_SERVER_PORT, SERVER_PORT_ENV, server_port


class ServerConfigTest(unittest.TestCase):
    def test_defaults_when_environment_is_missing(self):
        self.assertEqual(server_port({}), DEFAULT_SERVER_PORT)

    def test_reads_valid_environment_port(self):
        self.assertEqual(server_port({SERVER_PORT_ENV: "19090"}), 19090)

    def test_invalid_environment_port_falls_back(self):
        for value in ("invalid", "80", "70000", ""):
            with self.subTest(value=value):
                self.assertEqual(
                    server_port({SERVER_PORT_ENV: value}),
                    DEFAULT_SERVER_PORT,
                )


if __name__ == "__main__":
    unittest.main()
