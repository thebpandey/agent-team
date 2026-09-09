import pathlib
import sys

from playwright.sync_api import sync_playwright


snapshot = pathlib.Path(sys.argv[1]).resolve().as_uri()
evidence = pathlib.Path(sys.argv[2])
with sync_playwright() as playwright:
    browser = playwright.chromium.launch(headless=True)
    desktop = browser.new_page(viewport={"width": 1280, "height": 800})
    desktop.goto(snapshot)
    desktop.wait_for_load_state("networkidle")
    assert desktop.get_by_role("heading", name="All tasks").is_visible()
    assert desktop.locator("#task-rows tr").count() == 2
    desktop.get_by_label("Search tasks").fill("beta")
    assert desktop.locator("#task-rows tr:not([hidden])").count() == 1
    desktop.get_by_label("Status").select_option("completed")
    assert desktop.locator("#task-rows tr:not([hidden])").count() == 1
    desktop.get_by_role("button", name="Reload saved snapshot").click()
    desktop.wait_for_load_state("networkidle")
    assert desktop.get_by_role("group", name="Dependency graph", exact=True).is_visible()
    desktop.get_by_role("button", name="Filter task AT-ONE", exact=True).focus()
    desktop.keyboard.press("Enter")
    assert desktop.locator("#task-rows tr:not([hidden]) th").text_content() == "AT-ONE"
    desktop.get_by_label("Search tasks").fill("")
    desktop.get_by_role("button", name="Filter task AT-TWO", exact=True).click()
    assert desktop.locator("#task-rows tr:not([hidden]) th").text_content() == "AT-TWO"
    desktop.screenshot(path=str(evidence), full_page=True)
    mobile = browser.new_page(viewport={"width": 375, "height": 667})
    mobile.goto(snapshot)
    mobile.wait_for_load_state("networkidle")
    assert mobile.get_by_role("group", name="Dependency graph", exact=True).is_visible()
    mobile.get_by_role("button", name="Filter task AT-ONE", exact=True).click()
    assert mobile.locator("#task-rows tr:not([hidden]) th").text_content() == "AT-ONE"
    assert mobile.evaluate("document.documentElement.scrollWidth <= window.innerWidth")
    browser.close()
