// Offer a URL to Remote's page as a link click. An editor that intercepts
// such links claims it by stopping the event before it reaches the link.
function claimedAsLink(url, doc = document) {
  const link = doc.createElement("a");
  link.href = url;
  link.hidden = true;
  let unclaimed = false;
  link.addEventListener("click", (event) => {
    unclaimed = true;
    event.preventDefault();
  });
  doc.body.append(link);
  try {
    link.dispatchEvent(
      new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 }),
    );
  } finally {
    link.remove();
  }
  return !unclaimed;
}

function openTab(url) {
  const tab = window.open("", "_blank");
  if (!tab) return false;
  tab.opener = null;
  tab.location.replace(url);
  return true;
}

export const browserNavigation = {
  claimLink: claimedAsLink,
  openTab,
  download: (url) => window.location.assign(url),
};
