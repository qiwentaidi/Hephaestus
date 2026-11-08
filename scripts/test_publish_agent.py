"""Offline publication regression tests. No GitHub requests or Git mutations."""
import copy
from pathlib import Path
import tempfile
import subprocess
from unittest.mock import patch
import unittest
import zipfile

import publish_agent as release


class FakeGitHub:
    version = "v0.4.7"

    def __init__(self, dist, existing=True, corrupt=False):
        self.dist, self.corrupt, self.events = dist, corrupt, []
        self.state = self.empty() if existing else None
        if existing:
            self.state['assets'] = [{'name': release.LEGACY[0]}, {'name': 'unrelated.txt'}]
            self.state['draft'] = False

    def empty(self):
        return {'draft': True, 'assets': [], 'html_url': 'https://example.invalid/release'}

    def release(self):
        return copy.deepcopy(self.state)

    def command(self, action, *args):
        self.events.append((action, args))
        if action == 'create':
            self.state = self.empty()
        elif action == 'upload':
            for name in args:
                if name.startswith('--'):
                    continue
                path = Path(name)
                self.state['assets'] = [a for a in self.state['assets'] if a['name'] != path.name]
                digest = '0' * 64 if self.corrupt else release.sha256(path)
                self.state['assets'].append({'name': path.name, 'state': 'uploaded',
                                             'digest': 'sha256:' + digest, 'size': path.stat().st_size})
        elif action == 'delete-asset':
            self.state['assets'] = [a for a in self.state['assets'] if a['name'] != args[0]]
        elif action == 'edit' and '--draft=false' in args:
            self.state['draft'] = False


class PublicationTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.dist = Path(self.temp.name)
        packages = self.dist / 'packages'
        packages.mkdir()
        for name in release.PACKAGES:
            with zipfile.ZipFile(packages / name, 'w') as archive:
                archive.writestr('hephaestus-scan-agent' + ('.exe' if 'windows' in name else ''), b'fixture')
        self.rehash()
        (self.dist / 'installer-assets').mkdir()
        for name in release.INSTALLERS:
            (self.dist / 'installer-assets' / name).write_bytes(b'exported fixture')
        self.hashes = release.validate_artifacts(self.dist)

    def rehash(self):
        packages = self.dist / 'packages'
        (packages / 'SHA256SUMS.txt').write_text(''.join(
            f'{release.sha256(packages / name)}  {name}\n' for name in release.PACKAGES))

    def sync(self, github):
        def callback():
            release.verify_remote(github.release(), (*release.PACKAGES, 'SHA256SUMS.txt'),
                                  self.hashes, self.dist / 'packages')
            github.events.append(('push-installers', ()))
            return 'fixture-commit'
        return callback

    def test_existing_release_keeps_notes_and_removes_only_legacy_after_push(self):
        github = FakeGitHub(self.dist)
        release.publish(github, self.dist, self.hashes, self.sync(github), None)
        actions = [e[0] for e in github.events]
        self.assertLess(actions.index('push-installers'), actions.index('delete-asset'))
        self.assertNotIn('edit', actions)
        self.assertIn('unrelated.txt', {a['name'] for a in github.state['assets']})
        self.assertTrue((self.dist / 'release-before-v0.4.7.json').exists())
        github.events.clear()
        release.publish(github, self.dist, self.hashes, self.sync(github), None)
        uploads = [args for action, args in github.events if action == 'upload']
        self.assertEqual(len(uploads), 1)
        self.assertTrue(uploads[0][0].endswith('SHA256SUMS.txt'))

    def test_corrupt_remote_stops_before_checksum_push_or_delete(self):
        github = FakeGitHub(self.dist, corrupt=True)
        with self.assertRaisesRegex(RuntimeError, '远端文件校验失败'):
            release.publish(github, self.dist, self.hashes, self.sync(github), None)
        self.assertEqual([e[0] for e in github.events], ['upload'])

    def test_new_release_stays_draft_until_verified_and_installers_pushed(self):
        github = FakeGitHub(self.dist, existing=False)
        release.publish(github, self.dist, self.hashes, self.sync(github), None)
        actions = [e[0] for e in github.events]
        self.assertEqual(actions[0], 'create')
        self.assertLess(actions.index('push-installers'), actions.index('edit'))
        self.assertFalse(github.state['draft'])
        self.assertIn('--target', github.events[-1][1])

    def test_unpublished_draft_found_when_tag_endpoint_returns_404(self):
        github = release.GitHub('v0.4.7', {})
        missing = subprocess.CompletedProcess([], 1, '', 'gh: Not Found (HTTP 404)')
        with patch.object(release.subprocess, 'run', return_value=missing), patch.object(
                github, 'call', return_value='{"draft":true,"tag_name":"v0.4.7","assets":[]}\n'):
            self.assertTrue(github.release()['draft'])

    def test_failed_installer_push_prevents_cleanup_and_draft_publication(self):
        github = FakeGitHub(self.dist, existing=False)

        def failed_push():
            raise RuntimeError('fixture push failed')

        with self.assertRaisesRegex(RuntimeError, 'fixture push failed'):
            release.publish(github, self.dist, self.hashes, failed_push, None)
        self.assertTrue(github.state['draft'])
        self.assertNotIn('delete-asset', [e[0] for e in github.events])

    def test_bad_checksum_rejected(self):
        (self.dist / 'packages' / release.PACKAGES[0]).write_bytes(b'corrupted')
        with self.assertRaisesRegex(RuntimeError, '校验失败'):
            release.validate_artifacts(self.dist)

    def test_private_files_in_zip_rejected_even_with_valid_checksum(self):
        with zipfile.ZipFile(self.dist / 'packages' / release.PACKAGES[0], 'a') as archive:
            archive.writestr('config.yaml', b'private fixture')
        self.rehash()
        with self.assertRaisesRegex(RuntimeError, 'ZIP 内容不符合发布约定'):
            release.validate_artifacts(self.dist)

    def test_extra_package_rejected(self):
        (self.dist / 'packages/private.zip').write_bytes(b'fixture')
        with self.assertRaisesRegex(RuntimeError, '发布目录必须只包含'):
            release.validate_artifacts(self.dist)


if __name__ == '__main__':
    unittest.main()
