import { Composition } from "remotion";
import { AtlasDemo } from "./atlas-demo";

export function VideoRoot() {
  return (
    <Composition
      id="AtlasDemo"
      component={AtlasDemo}
      durationInFrames={1440}
      fps={24}
      width={1920}
      height={1080}
    />
  );
}
