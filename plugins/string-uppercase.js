// Example community plugin for m9m
module.exports = {
  description: {
    name: "string-uppercase",
    displayName: "String Uppercase",
    description: "Uppercases the `text` field of each item.",
    category: "Community",
    properties: [
      {displayName: "Field", name: "field", type: "string", default: "text", required: true}
    ],
    inputs:  ["main"],
    outputs: ["main"],
  },
  execute: function(inputData, nodeParams) {
    var field = nodeParams.field || "text";
    return inputData.map(function(item) {
      var v = item.json[field];
      var upper = (v == null) ? null : String(v).toUpperCase();
      var out = {};
      for (var k in item.json) out[k] = item.json[k];
      out[field] = upper;
      return { json: out };
    });
  },
};
