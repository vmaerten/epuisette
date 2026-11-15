import { Service } from "./bindings/github.com/vmaerten/video-downloader/index.js";

document.addEventListener("click", (event) => {
  const link = event.target.closest("[data-link]");
  if (link) Service.OpenLink(link.dataset.link);
});

const { version } = await Service.Config();
document.getElementById("version").textContent = `Version ${version}`;
