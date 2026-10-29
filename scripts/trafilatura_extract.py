#!/usr/bin/env python3
"""Trafilatura extraction wrapper for Bellkeeper.

Trafilatura 2.0 removed __main__.py, so `python3 -m trafilatura` no longer works.
This wrapper uses the Python API to achieve the same result.

Usage: python3 trafilatura_extract.py <url> [--timeout SECONDS] [--user-agent UA] [--headers JSON]
Output: JSON to stdout with {title, author, url, date, raw_text} or {error: ...}
"""
import sys
import json
from urllib.request import Request, urlopen
import trafilatura


def main():
    if len(sys.argv) < 2:
        print(json.dumps({"error": "URL argument required"}))
        sys.exit(1)

    url = sys.argv[1]
    timeout = 30
    user_agent = None
    extra_headers = {}
    for i, arg in enumerate(sys.argv):
        if arg == "--timeout" and i + 1 < len(sys.argv):
            timeout = int(sys.argv[i + 1])
        elif arg == "--user-agent" and i + 1 < len(sys.argv):
            user_agent = sys.argv[i + 1]
        elif arg == "--headers" and i + 1 < len(sys.argv):
            try:
                extra_headers = json.loads(sys.argv[i + 1])
            except json.JSONDecodeError:
                pass

    try:
        headers = extra_headers.copy() if extra_headers else {}
        if user_agent:
            headers["User-Agent"] = user_agent
        headers.setdefault("User-Agent", "Bellkeeper/2.4 (RSS article extraction)")
        # Trafilatura 2.0 fetch_url has no headers argument. Fetch explicitly so
        # domain overrides and the requested timeout both reach the HTTP client.
        with urlopen(Request(url, headers=headers), timeout=timeout) as response:
            downloaded = response.read(20 * 1024 * 1024 + 1)
        if len(downloaded) > 20 * 1024 * 1024:
            raise ValueError("response exceeds 20 MiB")
        if not downloaded:
            print(json.dumps({"error": "fetch failed", "url": url}))
            sys.exit(1)

        result = trafilatura.extract(
            downloaded,
            url=url,
            output_format="json",
            with_metadata=True,
            include_comments=False,
            include_tables=True,
            include_links=True,
        )
        if not result:
            print(json.dumps({"error": "extraction failed", "url": url}))
            sys.exit(1)

        sys.stdout.write(result)
    except Exception as e:
        print(json.dumps({"error": str(e), "url": url}))
        sys.exit(1)


if __name__ == "__main__":
    main()
