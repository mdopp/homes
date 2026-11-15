"""The colour check can go red (scripts/farben.py): a literal in a page, a
stale allowlist entry; and stays green on tokens, comments, tests and the
synced copy of the shared look."""
import os
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(__file__))
import farben  # noqa: E402


class Farben(unittest.TestCase):
    def repo(self, files):
        d = tempfile.mkdtemp()
        for path, text in files.items():
            os.makedirs(os.path.join(d, os.path.dirname(path)), exist_ok=True)
            with open(os.path.join(d, path), "w", encoding="utf-8") as f:
                f.write(text)
        subprocess.run(["git", "init", "-q", d], check=True)
        subprocess.run(["git", "-C", d, "add", "-A"], check=True)
        return d

    def test_a_literal_in_a_page_is_found(self):
        d = self.repo({"web/page.go": 'const css = `a{color:#f97316;background:rgba(0, 0, 0, .5)}`\n',
                       "web/x.svg": '<svg fill="hsl(20 90% 50%)"/>'})
        bad, stale = farben.check(d)
        self.assertEqual(bad, ["web/page.go:1: #f97316", "web/page.go:1: rgba(0,0,0,.5)",
                               "web/x.svg:1: hsl(2090%50%)"])
        self.assertEqual(stale, [])

    def test_tokens_comments_tests_and_the_shared_copy_pass(self):
        d = self.repo({"page.go": "// issue #376, laib#1170\nconst css = `a{color:var(--accent)}` // #fff\nvar link = `<a href=\"#top\">`\n",
                       "page_test.go": "const want = `#f97316`\n",
                       "static/gestaltung.css": ":root{--bg:#070709}\n",
                       "notes.md": "#fff\n",
                       "bild.go": "r, g, b, a := c.RGBA()\n"})
        self.assertEqual(farben.check(d), ([], []))

    def test_the_allowlist_names_user_content_and_goes_stale(self):
        d = self.repo({"cal.go": 'var farben = []string{"#7986cb", "#33b679"}\n',
                       farben.ALLOW: "# user-chosen calendar colours\ncal.go #7986cb #33B679 #000000  # Google's calendar colours\n"})
        bad, stale = farben.check(d)
        self.assertEqual(bad, [])
        self.assertEqual(stale, [f"{farben.ALLOW}: cal.go #000000 matches nothing any more: remove it"])


if __name__ == "__main__":
    unittest.main()
