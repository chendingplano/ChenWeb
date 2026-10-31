"""Browser contract checks. Run from any directory with --start-server.
Requires the project's Bun packages and Python Playwright/Chromium.
"""
import argparse
import json
import re
import socket
import subprocess
import tempfile
import time
from pathlib import Path
from playwright.sync_api import sync_playwright, expect


def check(url):
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1000, "height": 800})
        errors = []
        page.on("pageerror", lambda error: errors.append(str(error)))
        page.goto(url)
        page.wait_for_load_state("networkidle")
        roots = page.locator("[data-text-element]")
        expect(roots).to_have_count(6)
        assert page.locator("[data-flow-layout]").evaluate("e=>getComputedStyle(e).display") == "flex"
        expect(roots.nth(0).locator("p")).to_have_text("Review the customer details.\nSave after making changes.")
        expect(roots.nth(1).locator("p")).to_have_text("Old paragraph")
        expect(roots.nth(2).locator("p")).to_have_text("0")
        expect(roots.nth(3).locator("p")).to_have_text("false")
        expect(roots.nth(5).locator("p")).to_have_text("<script>escaped</script>")
        assert roots.nth(5).locator("script").count() == 0
        assert roots.nth(1).evaluate("e=>getComputedStyle(e).fontSize") == "14px"
        assert roots.nth(1).evaluate("e=>getComputedStyle(e).lineHeight") == "20px"
        assert all(roots.nth(i).evaluate("e=>getComputedStyle(e).userSelect") == "text" for i in range(6))
        assert roots.nth(0).locator(".icon").get_attribute("aria-hidden") == "true"
        help_button = page.get_by_role("button", name="More information", exact=True)
        help_button.focus()
        expect(page.get_by_role("tooltip")).to_be_visible()
        page.keyboard.press("Escape")
        expect(page.get_by_role("tooltip")).to_have_count(0)
        expect(help_button).to_be_focused()
        page.locator("#update").click()
        expect(roots.nth(2).locator("p")).to_have_text("42")
        # Hover transfers to selectable popup without collapsing it.
        help_button.hover()
        tip = page.get_by_role("tooltip")
        expect(tip).to_be_visible()
        tip.hover()
        page.wait_for_timeout(200)
        expect(tip).to_be_visible()
        page.locator("#update").hover()
        expect(tip).to_have_count(0, timeout=1000)
        # Switching locale from the keyboard keeps the popup open and repositions it.
        help_button.focus()
        page.keyboard.press("F2")
        expect(roots.nth(0).locator(".caption")).to_have_text("操作说明")
        expect(page.get_by_role("tooltip")).to_contain_text("中文帮助说明")
        rect = page.get_by_role("tooltip").bounding_box()
        assert rect and rect["x"] >= 0 and rect["y"] >= 0
        assert rect["x"] + rect["width"] <= 1000 and rect["y"] + rect["height"] <= 800
        page.keyboard.press("F2")
        # Hiding a cached page while a tooltip is open clears top-layer content.
        page.locator("#hide").evaluate("e=>e.click()")
        expect(page.get_by_role("tooltip")).to_have_count(0)
        page.locator("#hide").click()
        expect(page.get_by_role("tooltip")).to_have_count(0)
        help_button.focus()
        expect(page.get_by_role("tooltip")).to_be_visible()
        page.locator("#unmount").evaluate("e=>e.click()")
        expect(page.get_by_role("tooltip")).to_have_count(0)
        expect(roots).to_have_count(0)
        page.locator("#unmount").click()
        expect(roots).to_have_count(6)
        # Explicit-height content is contained and keyboard scrollable.
        scroll = roots.nth(4)
        assert scroll.evaluate("e=>e.scrollHeight>e.clientHeight")
        expect(scroll).to_have_attribute("tabindex", "0")
        assert scroll.bounding_box()["height"] == 48
        scroll.focus()
        page.keyboard.press("End")
        page.wait_for_timeout(100)
        assert scroll.evaluate("e=>e.scrollTop") > 0
        # Properties update the real renderer and preserve unsupported fields.
        page.locator("#text-prop-label_placement").select_option("left")
        first = roots.nth(0)
        expect(first.locator(".field")).to_have_class(re.compile(r".*\bhorizontal\b.*"))
        page.locator("#narrow").click()
        expect(first.locator(".field")).not_to_have_class(re.compile(r".*\bhorizontal\b.*"))
        page.locator("#narrow").click()
        page.locator("#text-prop-label_placement").select_option("right")
        expect(first.locator(".field")).to_have_class(re.compile(r"(?=.*\bhorizontal\b)(?=.*\breverse\b).*"))
        page.locator("#text-prop-label_placement").select_option("bottom")
        expect(first.locator(".field")).to_have_class(re.compile(r".*\breverse\b.*"))
        # Partial and empty lengths remain editable until focus leaves the field.
        width = page.locator('#text-prop-width')
        width.fill('200px')
        width.blur()
        width.focus()
        page.keyboard.press('End')
        page.keyboard.press('Backspace')
        expect(width).to_have_value('200p')
        expect(width).to_have_attribute('aria-invalid', 'false')
        page.keyboard.press('Backspace')
        expect(width).to_have_value('200')
        width.fill('250px')
        width.blur()
        assert json.loads(page.locator('#attrs').inner_text())['width'] == '250px'
        height = page.locator('#text-prop-height')
        height.fill('')
        expect(height).to_have_value('')
        height.fill('120px')
        height.blur()
        assert json.loads(page.locator('#attrs').inner_text())['height'] == '120px'
        width.fill('10vw')
        expect(width.locator('..').get_by_role('alert')).to_have_count(0)
        width.blur()
        expect(width.locator('..').get_by_role('alert')).to_contain_text('px, rem, em, or %')
        height.fill('10vh')
        height.blur()
        expect(height.locator('..').get_by_role('alert')).to_contain_text('px, rem, em, or %')
        height.fill('120px')
        height.blur()
        page.locator('#text-prop-max_width').fill('300px')
        page.locator('#text-prop-max_width').blur()
        width.fill('400px')
        expect(width.locator('..').get_by_role('alert')).to_have_count(0)
        width.blur()
        expect(width.locator('..').get_by_role('alert')).to_contain_text('maximum')
        width.fill('50%')
        width.blur()
        expect(width.locator('..').get_by_role('alert')).to_have_count(0)
        height.fill('160px')
        height.blur()
        expect(height.locator('..').get_by_role('alert')).to_contain_text('maximum')
        page.locator('#text-prop-max_height').fill('200px')
        page.locator('#text-prop-max_height').blur()
        expect(height.locator('..').get_by_role('alert')).to_have_count(0)
        # Invalid author values are validated on blur, then reset restores the default.
        size = page.locator("#text-prop-font_size")
        size.fill("0px")
        expect(size).to_have_attribute("aria-invalid", "false")
        size.blur()
        expect(size).to_have_attribute("aria-invalid", "true")
        size.fill("1rem")
        size.blur()
        expect(size).to_have_attribute("aria-invalid", "false")
        assert first.evaluate("e=>getComputedStyle(e).fontSize") == "16px"
        size.locator("..").get_by_role("button", name="Reset").click()
        state = json.loads(page.locator("#attrs").inner_text())
        assert "font_size" not in state and state["future_option"] == "preserved"
        expect(size).to_have_value("0.875rem")
        expect(page.locator("#text-prop-font_style")).to_have_attribute("readonly", "")
        expect(page.locator('#text-prop-future_option').locator('..').get_by_text("Unsupported by text; preserved with a warning.", exact=True)).to_be_visible()
        page.get_by_role("button", name="No tooltip", exact=True).click()
        assert json.loads(page.locator("#attrs").inner_text())["help_tip"] == ""
        expect(first.locator("button")).to_have_count(0)
        expect(page.get_by_role("textbox", name="Tooltip", exact=True)).to_have_value("")
        page.get_by_role("button", name="Use empty text", exact=True).click()
        expect(first.locator("p")).to_have_text("")
        expect(page.get_by_role("textbox", name="Text", exact=True)).to_have_value("")
        # Bilingual text editing and the dark theme retain selection and inherited colors.
        page.locator("#text-prop-text").locator("..").get_by_role("combobox").select_option("localized")
        page.locator("#text-prop-text-en").fill("English body")
        page.locator("#text-prop-text-zh-cn").fill("中文正文")
        page.keyboard.press("F2")
        expect(first.locator("p")).to_have_text("中文正文")
        page.locator("#theme").click()
        assert first.evaluate("e=>getComputedStyle(e).color") == "rgb(245, 245, 245)"
        page.screenshot(path="/tmp/wpi-text-r2-browser.png", full_page=True)
        assert not errors, errors
        # Touch has the same help and outside-dismiss affordance.
        touch = browser.new_context(has_touch=True, viewport={"width": 390, "height": 700})
        mobile = touch.new_page()
        mobile.goto(url)
        mobile.wait_for_load_state("networkidle")
        mobile.get_by_role("button", name="More information", exact=True).tap()
        expect(mobile.get_by_role("tooltip")).to_be_visible()
        mobile.locator("#update").tap()
        expect(mobile.get_by_role("tooltip")).to_have_count(0)
        touch.close()
        browser.close()
        print("PASS: real renderer/editor defaults, bindings, locale, escaped HTML, caption layout, tooltip keyboard/hover/hide/touch, selection, scrolling, reset/fixed/unsupported properties and themes")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--start-server", action="store_true")
    args = parser.parse_args()
    process = None
    with tempfile.TemporaryFile(mode="w+") as log:
        try:
            if args.start_server:
                web = Path(__file__).resolve().parents[2]
                process = subprocess.Popen([str(web / "node_modules/.bin/vite"), "--config", "tests/wpi-text/vite.config.ts"], cwd=web, stdout=log, stderr=log)
                deadline = time.monotonic() + 30
                while True:
                    try:
                        with socket.create_connection(("127.0.0.1", 5198), timeout=0.5):
                            break
                    except OSError:
                        if process.poll() is not None or time.monotonic() > deadline:
                            log.seek(0)
                            raise RuntimeError(log.read())
                        time.sleep(0.2)
            check("http://127.0.0.1:5198")
        finally:
            if process:
                process.terminate()
                process.wait(timeout=10)
