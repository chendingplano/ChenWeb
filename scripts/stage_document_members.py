#!/usr/bin/env python3
"""Stage Document Builder group members, preserving workspace-relative paths."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys


def main():
    workspace = os.environ.get('PROJECT_WORKSPACE_DIR', '')
    if not workspace or not Path(workspace).is_dir():
        raise ValueError('PROJECT_WORKSPACE_DIR must name an existing directory')
    root = Path(workspace).resolve()
    output = Path(sys.argv[1])
    env = os.environ.copy()
    # Match the application's PostgreSQL configuration; credentials stay in the environment.
    dotenv = Path(__file__).resolve().parents[1] / '.env'
    values = {}
    if dotenv.is_file():
        for line in dotenv.read_text().splitlines():
            line = line.strip().removeprefix('export ')
            if line and not line.startswith('#') and '=' in line:
                key, value = line.split('=', 1)
                values[key.strip()] = value.strip().strip("'\"")
    for pg, app in [('PGHOST', 'PG_HOST'), ('PGPORT', 'PG_PORT'), ('PGUSER', 'PG_USER_NAME'),
                    ('PGPASSWORD', 'PG_PASSWORD'), ('PGDATABASE', 'PG_DB_NAME')]:
        value = env.get(app) or values.get(app)
        if value:
            env[pg] = value
    result = subprocess.run(['psql', '-X', '-At', '-v', 'ON_ERROR_STOP=1', '-c',
                             "SELECT COALESCE(json_agg(row_to_json(m)), '[]'::json) FROM "
                             "(SELECT DISTINCT member_type, path FROM kb.doc_group_members ORDER BY path, member_type) m"],
                            env=env, check=True, capture_output=True, text=True)
    members = json.loads(result.stdout)
    if output.exists():
        shutil.rmtree(output)
    output.mkdir(parents=True)
    for member in members:
        path = Path(member['path'])
        if path.is_absolute() or '..' in path.parts or not path.parts:
            raise ValueError(f'invalid member path: {path}')
        source = root / path
        if not source.resolve().is_relative_to(root):
            raise ValueError(f'member escapes workspace: {path}')
        kind = member['member_type']
        if (kind == 'dir' and not source.is_dir()) or (kind == 'file' and not source.is_file()) or kind not in ('dir', 'file'):
            raise ValueError(f'missing or invalid {kind} member: {path}')
        destination = output / path
        if kind == 'dir':
            shutil.copytree(source, destination, dirs_exist_ok=True)
        else:
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, destination)
        print(f'Staged {kind}: {path}')


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as exc:
        if isinstance(exc, subprocess.CalledProcessError) and exc.stderr:
            print(exc.stderr.strip(), file=sys.stderr)
        print(f'Document staging failed: {exc}', file=sys.stderr)
        sys.exit(1)
