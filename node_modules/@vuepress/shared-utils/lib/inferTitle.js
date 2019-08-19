"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
const deeplyParseHeaders_1 = __importDefault(require("./deeplyParseHeaders"));
module.exports = function (frontmatter, strippedContent) {
    if (frontmatter.home) {
        return 'Home';
    }
    if (frontmatter.title) {
        return deeplyParseHeaders_1.default(frontmatter.title);
    }
    const match = strippedContent.trim().match(/^#+\s+(.*)/);
    if (match) {
        return deeplyParseHeaders_1.default(match[1]);
    }
};
