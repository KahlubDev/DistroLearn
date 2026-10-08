import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "DistroLearn",
  description: "Hands-on distributed-systems labs in the browser",
};

export default function HomePage() {
  return <h1>DistroLearn</h1>;
}
