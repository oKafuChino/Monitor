# Embedded frontend

The only build input is the checked-in komari-web directory in this repository. Do not clone an upstream frontend over local source.

Run from the repository root with Node.js 24+ and Go 1.25+ (CGO enabled):

```sh
npm ci --prefix komari-web
npm run test:onboarding --prefix komari-web
npm run build --prefix komari-web
node scripts/embed-frontend.mjs
go build .
```

The script requires tar, packs the real dist including index.html as tar+zstd, and copies komari-theme.json as the immutable UI settings schema. Generated archives are ignored by Git. CI uses .github/actions/build-frontend and restores the same frontend artifact for each backend target.

All pages and assets are served from this embedded build. Old data/theme and data/plugin directories are never served or executed. Site custom_head/custom_body and favicon remain supported; restricted setup/recovery pages continue to omit site injection and service-worker registration.
