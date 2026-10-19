import contextlib
import io
import subprocess
import unittest
from unittest.mock import patch

from ensure_release import ensure_release


def response(status, stderr=""):
    return subprocess.CompletedProcess(
        [], 0 if status == 200 else 1, f"HTTP/2.0 {status}\n", stderr,
    )


def missing_release():
    return [response(404), subprocess.CompletedProcess([], 0, "", "")]


class EnsureReleaseTests(unittest.TestCase):
    def setUp(self):
        output = contextlib.redirect_stdout(io.StringIO())
        output.__enter__()
        self.addCleanup(output.__exit__, None, None, None)
        runner = patch("ensure_release.subprocess.run")
        self.gh = runner.start()
        self.addCleanup(runner.stop)

    def test_existing_release_is_preserved_without_mutations(self):
        self.gh.return_value = response(200)
        ensure_release("v1.2.3", "notes.md")
        self.gh.assert_called_once()
        self.assertEqual(self.gh.call_args.args[0][1], "api")

    def test_missing_release_is_created_with_verified_tag_and_notes(self):
        self.gh.side_effect = [*missing_release(), response(200)]
        ensure_release("v1.2.3", "custom notes.md")
        command = self.gh.call_args.args[0]
        self.assertEqual(command[:4], ["gh", "release", "create", "v1.2.3"])
        self.assertIn("--verify-tag", command)
        self.assertEqual(command[command.index("--notes-file") + 1], "custom notes.md")
        self.assertIn("--generate-notes", command)
        self.assertNotIn("--prerelease", command)

    def test_existing_draft_is_preserved_without_publishing(self):
        self.gh.side_effect = [
            response(404), subprocess.CompletedProcess([], 0, "v2.0.0\nv1.2.3\n", ""),
        ]
        ensure_release("v1.2.3", "notes.md")
        self.assertEqual(self.gh.call_count, 2)
        self.assertTrue(all(call.args[0][1] == "api" for call in self.gh.call_args_list))
        self.assertIn("--paginate", self.gh.call_args.args[0])

    def test_draft_lookup_error_stops_before_creation(self):
        self.gh.side_effect = [response(404), response(403, "draft lookup failed")]
        with self.assertRaisesRegex(RuntimeError, "draft lookup failed"):
            ensure_release("v1.2.3", "notes.md")
        self.assertEqual(self.gh.call_count, 2)

    def test_prerelease_cannot_be_latest(self):
        self.gh.side_effect = [*missing_release(), response(200)]
        ensure_release("v1.2.3-rc.1", "notes.md")
        command = self.gh.call_args.args[0]
        self.assertIn("--prerelease", command)
        self.assertIn("--latest=false", command)

    def test_build_metadata_hyphen_does_not_make_prerelease(self):
        self.gh.side_effect = [*missing_release(), response(200)]
        ensure_release("v1.2.3+build-info", "notes.md")
        self.assertNotIn("--prerelease", self.gh.call_args.args[0])

    def test_rerunning_after_creation_does_not_create_again(self):
        self.gh.side_effect = [*missing_release(), response(200), response(200)]
        ensure_release("v1.2.3", "notes.md")
        ensure_release("v1.2.3", "notes.md")
        creates = [call for call in self.gh.call_args_list if call.args[0][1] == "release"]
        self.assertEqual(len(creates), 1)

    def test_concurrent_creation_is_successful_without_editing(self):
        self.gh.side_effect = [*missing_release(), response(422, "already exists"), response(200)]
        ensure_release("v1.2.3", "notes.md")
        self.assertEqual(self.gh.call_count, 4)
        self.assertFalse(any("edit" in call.args[0] for call in self.gh.call_args_list))

    def test_lookup_errors_are_not_mistaken_for_missing_releases(self):
        for status in (401, 403, 429, 500):
            with self.subTest(status=status):
                self.gh.reset_mock(side_effect=True)
                self.gh.return_value = response(status, "lookup failed")
                with self.assertRaisesRegex(RuntimeError, "Could not check release"):
                    ensure_release("v1.2.3", "notes.md")
                self.gh.assert_called_once()

    def test_network_error_stops_before_creation(self):
        self.gh.return_value = subprocess.CompletedProcess([], 1, "", "network unavailable")
        with self.assertRaisesRegex(RuntimeError, "network unavailable"):
            ensure_release("v1.2.3", "notes.md")
        self.gh.assert_called_once()

    def test_creation_failure_remains_a_failure_if_release_is_missing(self):
        self.gh.side_effect = [*missing_release(), response(403, "permission denied"), *missing_release()]
        with self.assertRaisesRegex(RuntimeError, "Could not create release.*permission denied"):
            ensure_release("v1.2.3", "notes.md")

    def test_tag_is_encoded_as_one_api_path_segment(self):
        self.gh.return_value = response(200)
        ensure_release("v1.2.3+build/meta", "notes.md")
        self.assertTrue(self.gh.call_args.args[0][-1].endswith("v1.2.3%2Bbuild%2Fmeta"))


if __name__ == "__main__":
    unittest.main()
