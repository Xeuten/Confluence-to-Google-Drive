package main

import (
	"confluence-to-google-drive/utils"
	"context"

	"google.golang.org/api/docs/v1"
)

func main() {
	doc := utils.GetDocument()
	tableTitle := doc.Find("h1#title-text").First().Children().Text()
	columnTitles := utils.GetColumnTitles(doc)
	var codes, descriptions []string
	utils.FillContents(&codes, &descriptions, doc)

	driveService, docService := utils.CreateServices(context.Background())
	fileId := utils.FileId(driveService, tableTitle)
	if fileId == "" {
		fileId = utils.CreateFile(driveService, tableTitle)
	} else {
		utils.AdjustCounter(2)
	}
	utils.ClearDocument(driveService, fileId)
	utils.UnwrapValue(docService.Documents.BatchUpdate(fileId, &docs.BatchUpdateDocumentRequest{
		Requests: utils.CreateEditRequests(columnTitles, codes, descriptions),
	}).Do())
}
