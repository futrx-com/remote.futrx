import type { ProjectTemplate } from "../../models/template";

/**
 * State of the new-project dialog. The template is chosen here and nowhere
 * else: a project's stack preset is immutable once its container exists, so
 * this dialog is the whole decision surface.
 */
export interface NewProjectState {
  open: boolean;
  name: string;
  /** Selected template name; "" until the catalog loads. */
  template: string;
  templates: ProjectTemplate[];
  templatesLoading: boolean;
  templatesError: string;
  submitting: boolean;
  error: string;
}

export type NewProjectAction =
  | { type: "open" }
  | { type: "close" }
  | { type: "set-name"; name: string }
  | { type: "select-template"; template: string }
  | { type: "templates-loading" }
  | { type: "templates-loaded"; templates: ProjectTemplate[] }
  | { type: "templates-failed"; error: string }
  | { type: "submit" }
  | { type: "submit-failed"; error: string };

/** The template assumed before the catalog answers, and if it omits a default. */
export const DEFAULT_TEMPLATE = "blank";

class NewProjectStateTransitions {
  createInitial(): NewProjectState {
    return {
      open: false,
      name: "",
      template: DEFAULT_TEMPLATE,
      templates: [],
      templatesLoading: false,
      templatesError: "",
      submitting: false,
      error: "",
    };
  }

  readonly reduce = (
    state: NewProjectState,
    action: NewProjectAction
  ): NewProjectState => {
    switch (action.type) {
      case "open":
        // Reopening starts from a clean form but keeps the already-loaded
        // catalog, so the picker does not flash on every open.
        return {
          ...state,
          open: true,
          name: "",
          error: "",
          submitting: false,
          template: this.defaultTemplate(state.templates),
        };
      case "close":
        return { ...state, open: false, submitting: false, error: "" };
      case "set-name":
        return { ...state, name: action.name, error: "" };
      case "select-template":
        return { ...state, template: action.template, error: "" };
      case "templates-loading":
        return { ...state, templatesLoading: true, templatesError: "" };
      case "templates-loaded":
        return {
          ...state,
          templatesLoading: false,
          templatesError: "",
          templates: action.templates,
          template: this.selectedOrDefault(state.template, action.templates),
        };
      case "templates-failed":
        // A failed catalog must not block project creation: the form falls
        // back to the default template, which is what every project got
        // before templates existed.
        return {
          ...state,
          templatesLoading: false,
          templatesError: action.error,
          templates: [],
          template: DEFAULT_TEMPLATE,
        };
      case "submit":
        return { ...state, submitting: true, error: "" };
      case "submit-failed":
        return { ...state, submitting: false, error: action.error };
    }
  };

  /** The template a freshly opened dialog starts on. */
  defaultTemplate(templates: ProjectTemplate[]): string {
    return templates.find((template) => template.default)?.name ?? DEFAULT_TEMPLATE;
  }

  /** Keeps a still-valid selection, otherwise falls back to the default. */
  selectedOrDefault(selected: string, templates: ProjectTemplate[]): string {
    if (templates.some((template) => template.name === selected)) return selected;
    return this.defaultTemplate(templates);
  }

  /** Trimmed project name, which is what gets submitted. */
  submittedName(state: NewProjectState): string {
    return state.name.trim();
  }

  canSubmit(state: NewProjectState): boolean {
    return !state.submitting && this.submittedName(state).length > 0;
  }

  selectedTemplate(state: NewProjectState): ProjectTemplate | undefined {
    return state.templates.find((template) => template.name === state.template);
  }

  /**
   * One-line warning about how long the first start will take. Only a
   * provisioning template without a published image pays the slow path.
   */
  firstStartNotice(state: NewProjectState): string {
    const template = this.selectedTemplate(state);
    if (!template || !template.provisions) return "";
    if (template.prebuiltImageAvailable) {
      return "This host has a prebuilt image for this template, so the container starts immediately.";
    }
    return "The stack is installed inside the container on first start, which can take several minutes.";
  }
}

export const newProjectState = new NewProjectStateTransitions();
