import pathlib
import sys

from playwright.sync_api import sync_playwright


with sync_playwright() as playwright:
    browser = playwright.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1280, "height": 800})
    page.goto("http://127.0.0.1:4173/")
    page.wait_for_load_state("networkidle")
    assert page.get_by_text("LOCAL LIVE STATUS").is_visible()
    assert page.get_by_text("live-1").is_visible()
    page.get_by_role("button", name="Refresh local status").click()
    page.wait_for_load_state("networkidle")
    assert page.get_by_text("live-2").is_visible()
    assert page.get_by_role("img", name="Interactive dependency graph. Use the task table for keyboard filtering and full dependency text.").is_visible()
    pathlib.Path(sys.argv[1]).parent.mkdir(parents=True, exist_ok=True)
    page.screenshot(path=sys.argv[1], full_page=True)
    browser.close()
