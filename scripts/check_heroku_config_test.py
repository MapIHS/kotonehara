import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("check_config", Path(__file__).with_name("check-heroku-config.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ConfigTest(unittest.TestCase):
    def test_disabled_rpg_does_not_require_rpg_secrets(self):
        module.validate({"DATABASE_URL": "postgres://example/db"})

    def test_persistent_db_required(self):
        for config in ({}, {"DATABASE_URL": "file:/app/data/bot.db", "DB_DRIVER": "sqlite"}):
            with self.assertRaises(ValueError):
                module.validate(config)

    def test_enabled_rpg_validates_origin_and_secret(self):
        config = {"DATABASE_URL": "postgres://example/db", "RPG_ENABLED": "true",
                  "RPG_PUBLIC_URL": "https://api.example.com", "RPG_GATEWAY_SECRET": "a" * 64}
        module.validate(config)
        for key, value in (("RPG_GATEWAY_SECRET", "short"), ("RPG_PUBLIC_URL", "http://100.89.85.96:1338"),
                           ("RPG_PUBLIC_URL", "https://api.example.com/rpg")):
            with self.subTest(key=key, value=value), self.assertRaises(ValueError):
                module.validate({**config, key: value})
