#!/usr/bin/env python3
"""How Pimpo talks to S3-compatible storage for cloud backups. Writes decisions_backup.json."""
import json
from prioritize import PRODUCT, call

CTX = (" Pimpo is a free, open-source single Go binary on the owner's computer; it must upload, list, download and delete"
       " encrypted backup files on Amazon S3 and S3-compatible services (Cloudflare R2, Backblaze B2, MinIO, Wasabi);"
       " maintainers value a small dependency tree, correctness of request signing, and easy testing with a fake server.")
QS = {"s3": {"type": "choice", "instructions": "Which S3 client approach should Pimpo use?" + CTX,
  "criteria": {
    "sdk_signer": "Plain net/http requests signed with the official AWS SDK v4 signer package only (aws-sdk-go-v2 core, no service clients)",
    "full_sdk": "The full AWS SDK for Go v2 S3 client (service/s3 with its transfer manager)",
    "minio_go": "The minio-go client library",
    "hand_sigv4": "A hand-written SigV4 signer with no new dependency"}},
  "drive_scope": {"type": "choice", "instructions": "Which Google Drive permission should Pimpo ask for to keep backups in the owner's Drive?" + CTX,
  "criteria": {
    "drive_file": "drive.file: only files Pimpo itself created, visible in a Pimpo folder in the owner's Drive",
    "drive_appdata": "drive.appdata: a hidden app folder the owner cannot see or download from in Drive",
    "drive_full": "drive: full access to every file in the owner's Drive"}}}
r = call({"product": PRODUCT}, QS)
json.dump(r, open("decisions_backup.json", "w"), indent=1)
for k in QS:
    a = r["answers"][k]
    print(k, a["choice"], sorted(((o, round(p, 2)) for o, p in a["probabilities"].items()), key=lambda x: -x[1]))
