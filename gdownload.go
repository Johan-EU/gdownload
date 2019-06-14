/**
 * Copyright Johan Boer
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package main

import (
	b64 "encoding/base64"
	"fmt"
	"google.golang.org/api/gmail/v1"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

var r *regexp.Regexp

func init() {
	r = regexp.MustCompile(`^(.*?)(?:\(([0-9]+)\))?(\.[^\.]*)?$`)
}

// Downloads all attachments from the messages of the given gmail query
func gmailDownloadAttachments(svc *gmail.Service, query string, outDir string) {
	totalMsg, totalAtt := 0, 0
	pageToken := ""
	for {
		req := svc.Users.Messages.List("me").Q(query)
		if pageToken != "" {
			req.PageToken(pageToken)
		}
		r, err := req.Do()
		if err != nil {
			log.Fatalf("Unable to retrieve messages: %v", err)
		}

		for _, m := range r.Messages {
			msg, err := svc.Users.Messages.Get("me", m.Id).Do()
			if err != nil {
				log.Fatalf("Unable to retrieve message %v: %v", m.Id, err)
			}

			subject := ""
			for _, h := range msg.Payload.Headers {
				if h.Name == "Subject" {
					subject = h.Value
					break
				}
			}
			totalMsg++
			log.Printf("Message #%v: %v", totalMsg, subject)

			// Download attachment here
			n := 1
			for _, part := range msg.Payload.Parts {
				if part.Filename != "" {
					totalAtt++
					attachmentBody, err := svc.Users.Messages.Attachments.Get("me", m.Id, part.Body.AttachmentId).Do()
					if err != nil {
						log.Fatalf("Error retrieving attachment with id %v", part.Body.AttachmentId)
					}
					data, err := b64.URLEncoding.DecodeString(attachmentBody.Data)
					if err != nil {
						log.Fatalf("Error decoding attachment: %v", err)
					}
					name := getUniqeFilename(outDir, part.Filename)
					fullName := filepath.Join(outDir, name)
					if err = ioutil.WriteFile(fullName, data, 0644); err != nil {
						log.Fatalf("Unable to write to file %v", fullName)
					}
					if err = os.Chtimes(fullName, time.Now(), time.Unix(msg.InternalDate/1000, 0)); err != nil {
						log.Fatalf("Cannot change timestamps of file %v: %v", fullName, err)
					}
					log.Printf("Message #%v attachment #%v: %v\n", totalMsg, n, name)
					n++
				}
			}
		}

		if r.NextPageToken == "" {
			break
		}
		pageToken = r.NextPageToken
	}
	log.Printf("Downloaded %v attachments from %v messages\n", totalAtt, totalMsg)
}

func getUniqeFilename(path, file string) string {
	_, err := os.Stat(filepath.Join(path, file))
	if os.IsNotExist(err) {
		return file
	}
	matches := r.FindStringSubmatch(file)
	if matches == nil || len(matches) != 4 {
		log.Fatalf("Unexpected number of matches in file name regular expression: %v", matches)
	}
	i := 0
	if matches[2] != "" {
		// Already a ([0-9]) before the extension or at the end of a filename without extension
		i, err = strconv.Atoi(matches[2])
		if err != nil {
			log.Fatalf("Unexpected result of ([0-9]) match in file name regular expression: %v", matches[2])
		}
	}
	return getUniqeFilename(path, fmt.Sprintf("%s(%v)%s", matches[1], i+1, matches[3]))
}
