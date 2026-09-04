import { render } from "preact";
import { App } from "./app/App";
import "./index.css";

const root = document.getElementById("root")!;
render(<App />, root);
