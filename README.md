This is a [Next.js](https://nextjs.org) project bootstrapped with [`create-next-app`](https://nextjs.org/docs/app/api-reference/cli/create-next-app).

> **Repository note:** the Next.js application is the Atlas web client reference implementation (backed by
> SQLite/Prisma demo data). The production backend is being built incrementally in Go as described in
> [`architecture.md`](./architecture.md) and [`roadmap.md`](./roadmap.md). Phase 1 (backend foundation) is
> implemented and under review; see "Backend (Go)" below.

## Getting Started (web client)

First, run the development server:

```bash
npm run dev
# or
yarn dev
# or
pnpm dev
# or
bun dev
```

Open [http://localhost:3000](http://localhost:3000) with your browser to see the result.

You can start editing the page by modifying `app/page.tsx`. The page auto-updates as you edit the file.

This project uses [`next/font`](https://nextjs.org/docs/app/building-your-application/optimizing/fonts) to automatically optimize and load [Geist](https://vercel.com/font), a new font family for Vercel.

## Learn More

To learn more about Next.js, take a look at the following resources:

- [Next.js Documentation](https://nextjs.org/docs) - learn about Next.js features and API.
- [Learn Next.js](https://nextjs.org/learn) - an interactive Next.js tutorial.

You can check out [the Next.js GitHub repository](https://github.com/vercel/next.js) - your feedback and contributions are welcome!

## Deploy on Vercel

The easiest way to deploy your Next.js app is to use the [Vercel Platform](https://vercel.com/new?utm_medium=default-template&filter=next.js&utm_source=create-next-app&utm_campaign=create-next-app-readme) from the creators of Next.js.

Check out our [Next.js deployment documentation](https://nextjs.org/docs/app/building-your-application/deploying) for more details.

## Backend (Go)

The production backend lives in [`backend/`](backend/README.md) and is built incrementally as described in
[`architecture.md`](architecture.md) and [`roadmap.md`](roadmap.md). Phase 1 (foundation) is complete; Phase 2
(identity, tenancy and security) is under review because managed OIDC (provisionally WorkOS) is still pending ADR-002 sandbox credentials. The backend
intentionally contains no business handlers yet.

```bash
cd backend
go run ./cmd/atlas-migrate status   # DATABASE_URL required
go run ./cmd/atlas-api
```

Toolchain requirements, environment variables, migration rules, the temporary sqlc configuration and test
instructions are documented in [`backend/README.md`](backend/README.md).
