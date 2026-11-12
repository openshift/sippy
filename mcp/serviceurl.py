"""Keep e2e database selection and credential redaction consistent with URI decoding."""

import argparse
import os
import re
import sys
from urllib.parse import unquote, urlsplit


def local_database_uri(uri: str, strip_overrides: bool = False) -> str:
    """Restrict destructive e2e operations to the URI's local host and database path."""
    try:
        parsed = urlsplit(uri)
        hostname = parsed.hostname
    except ValueError:
        raise ValueError("Malformed PostgreSQL URI authority") from None
    if parsed.scheme not in ("postgres", "postgresql"):
        raise ValueError("Unsupported PostgreSQL URI scheme")
    if hostname not in (
        "localhost", "sippy-postgres",
    ):
        raise ValueError("Non-local PostgreSQL URI")
    # Reject malformed ports and multi-host authorities as well.
    if "," in parsed.netloc.rsplit("@", 1)[-1]:
        raise ValueError("Multiple PostgreSQL hosts are not supported for e2e")
    try:
        parsed.port
    except ValueError:
        raise ValueError("Malformed PostgreSQL URI port") from None
    options = []
    for option in filter(None, parsed.query.split("&")):
        key, separator, _value = option.partition("=")
        if not separator:
            raise ValueError("Invalid PostgreSQL URI option")
        if unquote(key) in ("host", "hostaddr", "dbname", "service"):
            if not strip_overrides:
                raise ValueError("PostgreSQL URI overrides the e2e connection target")
            continue
        options.append(option)
    return parsed._replace(query="&".join(options)).geturl()


def redact_credentials(line: str) -> str:
    """Redact URI userinfo and decoded credential query keys without changing other text."""
    line = re.sub(r"(postgres(?:ql)?|rediss?)://[^/@\s]+@", r"\1://[redacted]@", line)

    def redact_option(match: re.Match) -> str:
        if unquote(match[2]) in ("password", "sslpassword"):
            return f"{match[1]}{match[2]}=[redacted]"
        return match[0]

    return re.sub(r"([?&])([^=&\s]+)=([^&\s]*)", redact_option, line)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("redact", "validate", "database"))
    command = parser.parse_args().command
    if command == "redact":
        for line in sys.stdin:
            sys.stdout.write(redact_credentials(line))
            sys.stdout.flush()
    else:
        try:
            uri = local_database_uri(os.environ["SIPPY_DATABASE_DSN"], command == "database")
        except KeyError:
            print("FATAL: SIPPY_DATABASE_DSN is required", file=sys.stderr)
            sys.exit(1)
        except ValueError as exc:
            print(f"FATAL: refusing unsafe e2e database connection settings: {exc}", file=sys.stderr)
            sys.exit(1)
        if command == "database":
            print(uri)
