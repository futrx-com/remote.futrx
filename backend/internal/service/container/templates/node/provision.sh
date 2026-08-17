# Node 22, npm and npx are already in the base image. Corepack ships with
# Node, so enabling pnpm/yarn costs no network round-trip to apt.
echo "--- enabling corepack package managers ---"
corepack enable
corepack prepare pnpm@latest --activate
corepack prepare yarn@stable --activate

echo "--- versions ---"
node --version
npm --version
pnpm --version
yarn --version
