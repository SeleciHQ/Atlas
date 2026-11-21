import { resetAndSeed } from "@/lib/demo-data";

resetAndSeed()
  .then(() => {
    console.log("Atlas demo data seeded.");
    process.exit(0);
  })
  .catch((err) => {
    console.error(err);
    process.exit(1);
  });
