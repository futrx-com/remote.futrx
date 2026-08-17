# Ubuntu 24.04 ships Python 3.12 as the system interpreter; the base image
# already pulled python3-pip. Add the venv module and pipx.
echo "--- installing python tooling ---"
apt_retry update -qq
apt_retry install -y -qq python3.12-venv python3-dev pipx

# uv: fast resolver/installer, used by most modern Python projects. Installed
# to /usr/local/bin so every shell in the container sees it.
echo "--- installing uv ---"
export UV_INSTALL_DIR=/usr/local/bin
export INSTALLER_NO_MODIFY_PATH=1
curl -fsSL https://astral.sh/uv/install.sh | sh

# A project virtualenv in the durable workspace, so it survives container
# replacement along with the rest of /workspace.
if [ ! -d /workspace/.venv ]; then
  echo "--- creating /workspace/.venv ---"
  python3 -m venv /workspace/.venv
  /workspace/.venv/bin/python -m pip install --quiet --upgrade pip
fi

echo "--- versions ---"
python3 --version
/workspace/.venv/bin/python --version
/usr/local/bin/uv --version
pipx --version
