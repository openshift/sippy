"""Keep e2e database selection and credential redaction consistent with URI decoding."""

import os
import re
import sys
from urllib.parse import unquote, urlsplit


def local_database_uri(uri: str, strip_overrides: bool = False) -> str:
    """Restrict destructive e2e operations to the URI's local host and database path."""
    parsed = urlsplit(uri)
    if parsed.scheme not in ("postgres", "postgresql") or parsed.hostname not in (
        "localhost", "sippy-postgres",
    ):
        raise ValueError("Non-local PostgreSQL URI")
    # Reject malformed ports and multi-host authorities as well.
    if "," in parsed.netloc.rsplit("@", 1)[-1]:
        raise ValueError("Multiple PostgreSQL hosts are not supported for e2e")
    parsed.port
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
    if sys.argv[1] == "redact":
        for line in sys.stdin:
            sys.stdout.write(redact_credentials(line))
            sys.stdout.flush()
    else:
        try:
            uri = local_database_uri(os.environ["SIPPY_DATABASE_DSN"], sys.argv[1] == "database")
        except (ValueError, KeyError):
            print("FATAL: refusing unsafe e2e database connection settings", file=sys.stderr)
            sys.exit(1)
        if sys.argv[1] == "database":
            print(uri)
