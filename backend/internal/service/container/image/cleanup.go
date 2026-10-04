package image

// Only run this in the disposable image builder, after every installer. In
// particular, /root/.cache/ms-playwright contains the installed browser, not
// disposable downloads. Do not clear provider homes or the whole .cache tree.
const cleanupScript = `set -eu
apt-get -o DPkg::Lock::Timeout=300 clean
find /var/lib/apt/lists -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
npm cache clean --force
rm -rf -- /root/.npm/_npx /root/.npm/_logs /root/.cache/pip
rm -rf -- /tmp/pw-vendor /tmp/code-server.deb
find /usr/local/bin -maxdepth 1 -type f -name 'agy*.old' -delete
`
