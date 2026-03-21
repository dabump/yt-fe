#!/bin/sh
set -e

cat > /app/config.yaml << EOF
video_dir: "${VIDEO_DIR:-video}"
thumbnails_dir: "${THUMBNAILS_DIR:-thumbnails}"
metadata_dir: "${METADATA_DIR:-metadata}"
static_dir: "${STATIC_DIR:-static}"
EOF

exec "$@"
