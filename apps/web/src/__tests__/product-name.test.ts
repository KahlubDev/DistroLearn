import { describe, expect, it } from "vitest";

import { PRODUCT_NAME } from "../app/product-name";

describe("PRODUCT_NAME", () => {
  it("is DistroLearn", () => {
    expect(PRODUCT_NAME).toBe("DistroLearn");
  });
});
