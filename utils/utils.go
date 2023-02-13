package utils

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	CredsFileName = "a41917258434652729376693285615-921be0244f15.json"
)

var errors = [10]string{
	"Failed to make a request to target URL (%v)\n",
	"Data cannot be parsed as html (%v)\n",
	"Unable to read credentials file: %v\n",
	"Unable to create credentials: %v\n",
	"Error creating Drive service: %v\n",
	"Error creating Docs service: %v\n",
	"Unable to retrieve files: %v\n",
	"Error creating document: %v\n",
	"Unable to create permission for file: %v\n",
	"Failed to update the document (%v\n)",
}

var errorCounter = 0

// GetDocument Эта функция считывает html-документ по известному URL'у.
func GetDocument() *goquery.Document {
	response := UnwrapValue(http.Get("https://confluence.hflabs.ru/pages/viewpage.action?pageId=1181220999"))
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Fatalf("Failed to close the connection: %v\n", err)
		}
	}(response.Body)
	return UnwrapValue(goquery.NewDocumentFromReader(response.Body))
}

// GetColumnTitles Эта функция считывает названия столбцов таблицы в
// массив из двух элементов.
func GetColumnTitles(doc *goquery.Document) []string {
	var columnTitles []string
	GetChildrenOfFirst(doc.Find("thead"), 2).Each(func(_ int, selection *goquery.Selection) {
		columnTitles = append(columnTitles, selection.Text())
	})
	return columnTitles
}

// FillContents Эта функция заполняет массивы codes и descriptions текстом
// из ячеек таблицы.
func FillContents(codes *[]string, descriptions *[]string, doc *goquery.Document) {
	counter := 0
	doc.Find("tbody").Children().Each(func(_ int, selection *goquery.Selection) {
		selection.Children().Each(func(_ int, selection2 *goquery.Selection) {
			if selection2.Children().Length() != 0 {
				if selection2.ChildrenFiltered("ul").Length() != 0 {
					*descriptions = append(*descriptions, CreateBulletList(selection2))
				} else {
					*descriptions = append(*descriptions, selection2.Text())
				}
			} else if counter%2 == 0 {
				*codes = append(*codes, selection2.Text())
			} else {
				*descriptions = append(*descriptions, selection2.Text())
			}
			counter++
		})
	})
}

// CreateClient Эта функция получает из контекста реквизиты и на их основе
// создаёт http-клиент.
func CreateClient(ctx context.Context) *http.Client {
	return oauth2.NewClient(ctx,
		UnwrapValue(google.CredentialsFromJSON(ctx,
			UnwrapValue(os.ReadFile(CredsFileName)), drive.DriveScope)).TokenSource)
}

// CreateServices Эта функция создаёт сервисы для взаимодействия с API
// Google Drive'а и Google Doc'ов.
func CreateServices(ctx context.Context) (*drive.Service, *docs.Service) {
	client := CreateClient(ctx)
	return UnwrapValue(drive.NewService(ctx, option.WithHTTPClient(client))),
		UnwrapValue(docs.NewService(ctx, option.WithHTTPClient(client)))
}

// FileId Эта функция проверят список доступных сервису файлов на наличие
// в нём файла с заданным именем. Если он есть в списке, то возвращает его
// id. Если он отсутствует, то возвращает пустую строку.
func FileId(driveService *drive.Service, fileName string) string {
	fileList := UnwrapValue(driveService.Files.List().Fields("nextPageToken, files(id, name)").Do()).Files
	for _, i := range fileList {
		if i.Name == fileName {
			return i.Id
		}
	}
	return ""
}

// CreateFile Эта функция с помощью сервиса создаёт новый файл с заданным
// именем и возвращает его id.
func CreateFile(driveService *drive.Service, fileName string) string {
	file := UnwrapValue(driveService.Files.Create(&drive.File{
		Name:     fileName,
		MimeType: "application/vnd.google-apps.document",
	}).Do())
	UnwrapValue(driveService.Permissions.Create(file.Id, &drive.Permission{
		Type: "anyone",
		Role: "reader",
	}).Do())
	return file.Id
}

// AdjustCounter Эта функция поправляет значение счётчика ошибок.
func AdjustCounter(value int) {
	errorCounter += value
}

// ClearDocument Эта функция с помощью сервиса стирает содержимое документа
// c заданным id.
func ClearDocument(driveService *drive.Service, fileId string) {
	emptyContent := ""
	UnwrapValue(driveService.Files.Update(fileId, &drive.File{}).
		Media(strings.NewReader(emptyContent), googleapi.ContentType("text/plain")).
		Do())
	errorCounter--
}

// CreateEditRequests Эта функция заполняет массив запросов на
// редактирование документа таким образом, чтобы данные из Confluence-
// таблицы перенеслись в Google-документ в корректном виде.
func CreateEditRequests(columnTitles []string, codes []string, descriptions []string) []*docs.Request {
	requests := []*docs.Request{
		{
			InsertTable: &docs.InsertTableRequest{
				EndOfSegmentLocation: &docs.EndOfSegmentLocation{
					SegmentId: "",
				},
				Columns: 2,
				Rows:    int64(len(codes)) + 1,
			},
		},
	}
	for i, j := 7+len(codes)*5, len(codes)-1; i > 7; i, j = i-5, j-1 {
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Text: descriptions[j],
				Location: &docs.Location{
					Index: int64(i),
				},
			},
		})
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Text: codes[j],
				Location: &docs.Location{
					Index: int64(i - 2),
				},
			},
		})
	}
	requests = append(requests, &docs.Request{
		InsertText: &docs.InsertTextRequest{
			Text: columnTitles[1],
			Location: &docs.Location{
				Index: 7,
			},
		},
	},
		&docs.Request{
			InsertText: &docs.InsertTextRequest{
				Text: columnTitles[0],
				Location: &docs.Location{
					Index: 5,
				},
			},
		},
	)
	return requests
}

// GetChildrenOfFirst Эта функция получает выборку из дочерних элементов
// первого элемента выборки, переданной в качестве аргумента, и далее
// рекурсивно продолжает тот же процесс заданное число раз.
func GetChildrenOfFirst(s *goquery.Selection, level int) *goquery.Selection {
	if level == 1 {
		return s.First().Children()
	} else {
		return GetChildrenOfFirst(s.First().Children(), level-1)
	}
}

// CreateBulletList Эта функция форматирует строку, обозначающую маркированный
// список, в специальном формате (сначала находит среди тегов часть,
// обозначающую начало списка, затем последовательно присоединяет к ней
// части текста, обозанчающие пункты списка, разделяя их переносами строк
// и вставляя знаки "-" подобно markdown формату).
func CreateBulletList(s *goquery.Selection) string {
	ul := s.ChildrenFiltered("ul")
	var bulletList bytes.Buffer
	bulletList.WriteString(s.ChildrenFiltered("p").Text() + "\n")
	ul.ChildrenFiltered("li").ChildrenFiltered("span").Each(func(_ int, selection *goquery.Selection) {
		bulletList.WriteString("- " + selection.Text() + "\n")
	})
	return bulletList.String()
}

// UnwrapValue Эта функция разворачивает пару (значение, ошибка), которая
// возвращается многим функциями в сам объект, если ошибки не произошло.
// В случае если произошла ошибка, ошибки обрабатываются универсальным
// образом с помощью счётчик, который позволяет определить код данной
// конкретной ошибки.
func UnwrapValue[V any](value V, err error) V {
	if err != nil {
		log.Fatalf(errors[errorCounter], err)
	}
	errorCounter++
	return value
}
