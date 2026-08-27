// Preact→React shim: the app is written in Preact, but Claude Design renders
// React. Only the APIs the UI code actually uses are re-exported; all have
// identical React signatures.
export {
  createContext,
  Fragment,
  Component,
  createElement as h,
  createRef,
  isValidElement,
  cloneElement,
} from "react";
