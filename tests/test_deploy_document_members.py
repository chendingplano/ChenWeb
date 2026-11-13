import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'scripts' / 'stage_document_members.py'


class StageMembersTest(unittest.TestCase):
    def test_files_directories_and_invalid_paths(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / 'workspace'
            (root / 'docs' / 'nested').mkdir(parents=True)
            (root / 'docs' / 'nested' / 'a.md').write_text('a')
            (root / 'single file.pdf').write_text('b')
            output = Path(tmp) / 'out'
            fake = Path(tmp) / 'psql'
            fake.write_text('#!/bin/sh\nprintf "%s\\n" "$TEST_MEMBERS"\n')
            fake.chmod(0o755)
            env = dict(os.environ, PATH=tmp + os.pathsep + os.environ['PATH'], PROJECT_WORKSPACE_DIR=str(root),
                       TEST_MEMBERS=json.dumps([{'member_type': 'dir', 'path': 'docs'}, {'member_type': 'file', 'path': 'single file.pdf'}]))
            result = subprocess.run(['python3', str(SCRIPT), str(output)], env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((output / 'docs/nested/a.md').read_text(), 'a')
            self.assertEqual((output / 'single file.pdf').read_text(), 'b')
            env['TEST_MEMBERS'] = json.dumps([{'member_type': 'file', 'path': '../outside'}])
            self.assertNotEqual(subprocess.run(['python3', str(SCRIPT), str(output)], env=env, capture_output=True).returncode, 0)
            env.pop('PROJECT_WORKSPACE_DIR')
            self.assertNotEqual(subprocess.run(['python3', str(SCRIPT), str(output)], env=env, capture_output=True).returncode, 0)


if __name__ == '__main__':
    unittest.main()
