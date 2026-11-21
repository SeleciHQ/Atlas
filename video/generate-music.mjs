import { writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

const SAMPLE_RATE = 44_100;
const DURATION_SECONDS = 60;
const TOTAL_SAMPLES = SAMPLE_RATE * DURATION_SECONDS;
const OUTPUT = fileURLToPath(new URL("../public/atlas-demo-ambient.wav", import.meta.url));
const CHORDS = [
  [110, 164.81, 220, 261.63],
  [87.31, 130.81, 174.61, 220],
  [130.81, 164.81, 196, 261.63],
  [98, 146.83, 196, 246.94],
];
const MELODY = [440, 392, 329.63, 392, 493.88, 440, 392, 329.63];

const data = Buffer.alloc(TOTAL_SAMPLES * 2);
for (let i = 0; i < TOTAL_SAMPLES; i += 1) {
  const t = i / SAMPLE_RATE;
  const chordIndex = Math.floor(t / 8) % CHORDS.length;
  const chordProgress = (t % 8) / 8;
  const chord = CHORDS[chordIndex];

  let sample = 0;
  for (let n = 0; n < chord.length; n += 1) {
    const frequency = chord[n];
    const phase = t * frequency * Math.PI * 2;
    const slowBreath = 0.8 + Math.sin(t * Math.PI * 0.18 + n * 0.75) * 0.2;
    const chordAttack = Math.min(1, chordProgress * 8);
    sample += (Math.sin(phase) + Math.sin(phase * 2) * 0.12) * slowBreath * chordAttack * 0.055;
  }

  const melodySlot = Math.floor(t / 3.2);
  const melodyTime = t % 3.2;
  const melodyEnvelope = Math.min(1, melodyTime * 1.8) * Math.max(0, 1 - melodyTime / 3.2);
  const melodyFrequency = MELODY[melodySlot % MELODY.length];
  sample += Math.sin(t * melodyFrequency * Math.PI * 2) * melodyEnvelope * 0.025;

  const fadeIn = Math.min(1, t / 2.5);
  const fadeOut = Math.min(1, (DURATION_SECONDS - t) / 3.5);
  const output = Math.max(-0.2, Math.min(0.2, sample * fadeIn * fadeOut));
  data.writeInt16LE(Math.round(output * 32767), i * 2);
}

const header = Buffer.alloc(44);
header.write("RIFF", 0);
header.writeUInt32LE(36 + data.length, 4);
header.write("WAVE", 8);
header.write("fmt ", 12);
header.writeUInt32LE(16, 16);
header.writeUInt16LE(1, 20);
header.writeUInt16LE(1, 22);
header.writeUInt32LE(SAMPLE_RATE, 24);
header.writeUInt32LE(SAMPLE_RATE * 2, 28);
header.writeUInt16LE(2, 32);
header.writeUInt16LE(16, 34);
header.write("data", 36);
header.writeUInt32LE(data.length, 40);

await writeFile(OUTPUT, Buffer.concat([header, data]));

const CLICK_DURATION = 0.085;
const CLICK_SAMPLES = Math.round(SAMPLE_RATE * CLICK_DURATION);
const clickData = Buffer.alloc(CLICK_SAMPLES * 2);
for (let i = 0; i < CLICK_SAMPLES; i += 1) {
  const t = i / SAMPLE_RATE;
  const attack = Math.min(1, t * 900);
  const body = Math.sin(2 * Math.PI * 1480 * t) * Math.exp(-t * 76);
  const tick = Math.sin(2 * Math.PI * 2900 * t) * Math.exp(-t * 145) * 0.28;
  const low = Math.sin(2 * Math.PI * 520 * t) * Math.exp(-t * 54) * 0.32;
  const sample = Math.max(-0.2, Math.min(0.2, (body + tick + low) * attack * 0.16));
  clickData.writeInt16LE(Math.round(sample * 32767), i * 2);
}

const clickHeader = Buffer.alloc(44);
clickHeader.write("RIFF", 0);
clickHeader.writeUInt32LE(36 + clickData.length, 4);
clickHeader.write("WAVE", 8);
clickHeader.write("fmt ", 12);
clickHeader.writeUInt32LE(16, 16);
clickHeader.writeUInt16LE(1, 20);
clickHeader.writeUInt16LE(1, 22);
clickHeader.writeUInt32LE(SAMPLE_RATE, 24);
clickHeader.writeUInt32LE(SAMPLE_RATE * 2, 28);
clickHeader.writeUInt16LE(2, 32);
clickHeader.writeUInt16LE(16, 34);
clickHeader.write("data", 36);
clickHeader.writeUInt32LE(clickData.length, 40);
const CLICK_OUTPUT = fileURLToPath(new URL("../public/atlas-ui-click.wav", import.meta.url));
await writeFile(CLICK_OUTPUT, Buffer.concat([clickHeader, clickData]));

console.log(`Generated original ambient music: ${path.basename(OUTPUT)}`);
console.log(`Generated synchronized UI click: ${path.basename(CLICK_OUTPUT)}`);
