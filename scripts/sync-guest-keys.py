#!/usr/bin/env python3
"""Update managed guest key blocks without rewriting YAML or its comments."""
import argparse
import base64
from pathlib import Path
import re
import struct

root = Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--check', action='store_true', help='fail if generated key blocks differ')
args = parser.parse_args()
keys = []
seen = set()
for line in (root / 'config/guest-ssh-keys.pub').read_text().splitlines():
    line = line.strip()
    if not line or line.startswith('#'):
        continue
    parts = line.split()
    if len(parts) < 2:
        raise SystemExit('Invalid public key')
    blob = base64.b64decode(parts[1], validate=True)
    size = struct.unpack('>I', blob[:4])[0]
    if blob[4:4+size].decode() != parts[0]:
        raise SystemExit('Public key type does not match its payload')
    if blob not in seen:
        keys.append(line)
        seen.add(blob)
if not keys:
    raise SystemExit('Guest access key list cannot be empty')
pattern = re.compile(r'(?m)^( +)# BEGIN MANAGED SSH KEYS\n.*?^\1# END MANAGED SSH KEYS', re.S)
changed = []
for path in sorted([*root.glob('cloudinit/*.yaml'), *root.glob('ignition/*.bu')]):
    original = path.read_text()
    updated, count = pattern.subn(lambda m: m[1]+'# BEGIN MANAGED SSH KEYS\n'+''.join(m[1]+'- '+key+'\n' for key in keys)+m[1]+'# END MANAGED SSH KEYS', original)
    if count != 2:
        raise SystemExit(f'{path.name}: expected two managed users; found {count}')
    if updated != original:
        changed.append(str(path.relative_to(root)))
        if not args.check:
            path.write_text(updated)
if args.check and changed:
    raise SystemExit('Run make guest-keys: '+', '.join(changed))
print('Guest keys synchronized' if not args.check else 'Guest keys match the central list')
