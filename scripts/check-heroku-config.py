"""Validate Heroku config streamed on stdin without printing secret values."""
import json
import sys
from urllib.parse import urlsplit


def validate(config):
    if not str(config.get("DATABASE_URL", "")).strip():
        raise ValueError("DATABASE_URL must reference the existing persistent PostgreSQL database")
    driver = str(config.get("DB_DRIVER", "postgres")).strip().lower()
    if driver and not (driver.startswith("postgres") or driver in ("pg", "pgx")):
        raise ValueError("Heroku requires DB_DRIVER=postgres; local SQLite is not persistent")
    enabled = str(config.get("RPG_ENABLED", "")).strip().lower() in ("1", "true", "yes", "y", "on")
    if enabled:
        origin = urlsplit(str(config.get("RPG_PUBLIC_URL", "")).strip())
        if (origin.scheme != "https" or not origin.hostname or origin.username is not None
                or origin.password is not None or origin.path not in ("", "/")
                or origin.query or origin.fragment):
            raise ValueError("RPG_PUBLIC_URL must be the HTTPS Hararest origin without /rpg")
        secret = str(config.get("RPG_GATEWAY_SECRET", "")).strip()
        if len(secret) < 32 or any(c.isspace() for c in secret):
            raise ValueError("RPG_GATEWAY_SECRET needs at least 32 characters without whitespace")


if __name__ == "__main__":
    try:
        validate(json.load(sys.stdin))
    except (ValueError, TypeError, AttributeError):
        # Parsing exceptions can include input. Never echo the incoming config.
        sys.exit("Invalid Heroku configuration: check DATABASE_URL, DB_DRIVER=postgres and (when RPG is enabled) its HTTPS public origin and gateway secret. See docs/heroku-rpg.md.")
    print("Heroku configuration passed (secret values omitted)")
