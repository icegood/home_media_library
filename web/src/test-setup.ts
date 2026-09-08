import "@testing-library/jest-dom/vitest";
import {configure} from "@testing-library/react";

// On loaded CI runners the React commit after an event can exceed the 1s
// default, which made timer-impatient waitFor assertions flaky. Give every
// async query a generous budget; passing assertions still resolve on the
// first poll, so suite runtime is unaffected in the green case.
configure({asyncUtilTimeout: 5000});
