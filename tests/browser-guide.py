"""Explicit maintainer browser qualification, not a production dependency.

Run with an installed Python Playwright package and browser. Evidence stays in
the supplied local output directory. No remote sites, host CLIs or tracker writes.
"""
import argparse
import json
from pathlib import Path

from playwright.sync_api import sync_playwright, expect


def run(guide: Path, output: Path):
    output.mkdir(parents=True, exist_ok=True)
    results = []
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            for width, height in [(1440, 1000), (375, 812), (812, 375)]:
                context = browser.new_context(
                    viewport={"width": width, "height": height},
                    reduced_motion="reduce",
                )
                page = context.new_page()
                errors = []
                remote_requests = []
                page.on("pageerror", lambda error: errors.append(str(error)))
                page.on("request", lambda request: remote_requests.append(request.url)
                        if request.url.startswith(("http://", "https://")) else None)
                page.goto(guide.as_uri(), wait_until="networkidle")
                page.screenshot(path=str(output / f"guide-viewport-{width}x{height}.png"))
                assert page.title() == "Project Kickoff + Agent-Team | First-time setup"
                assert page.locator("#kickoff").evaluate(
                    "el => !!(el.compareDocumentPosition(document.querySelector('#install')) & Node.DOCUMENT_POSITION_FOLLOWING)"
                ), "Project Kickoff must precede Agent-Team"
                assert page.locator("body").evaluate(
                    "el => document.documentElement.scrollWidth <= window.innerWidth"
                ), "Document overflows viewport"
                for section in ["kickoff", "install", "hooks", "setup", "settings", "start", "dashboard", "recovery"]:
                    summary = page.locator(f"#{section} > summary")
                    summary.focus()
                    summary.press("Enter")
                    assert page.locator(f"#{section}").get_attribute("open") is not None
                    assert page.locator("body").evaluate(
                        "el => document.documentElement.scrollWidth <= window.innerWidth"
                    ), f"Open {section} section overflows viewport"
                    summary.press("Enter")
                    assert page.locator(f"#{section}").get_attribute("open") is None
                page.locator("#setup > summary").click()
                assert page.locator("#setup pre").last.is_visible()
                page.locator("#setup .copy").last.click()
                expect(page.locator("#copy-status")).to_contain_text("cop", ignore_case=True)
                assert not errors, errors
                assert not remote_requests, remote_requests
                page.screenshot(path=str(output / f"guide-{width}x{height}.png"), full_page=True)
                page.locator("#setup > summary").click()
                page.locator("#dashboard > summary").click()
                expect(page.locator("#dashboard .flow li")).to_have_count(3)
                page.locator("#dashboard").screenshot(path=str(output / f"guide-dashboard-{width}x{height}.png"))
                results.append({"viewport": [width, height], "status": "passed", "consoleErrors": errors,
                                "remoteRequests": remote_requests})
                context.close()
        finally:
            browser.close()
    report = {"kind": "actual-playwright-browser", "guide": str(guide), "results": results}
    (output / "guide-browser.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--guide", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    run(args.guide.resolve(strict=True), args.output.resolve())
